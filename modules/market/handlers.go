package market

import (
	"bytes"
	"math"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/coinexchain/dex/modules/market/match"
	"github.com/coinexchain/dex/types"
)

const (
	MinTokenPricePrecision           = 8
	MaxTokenPricePrecision           = 18
	LimitOrder             OrderType = 2
	SymbolSeparator                  = "/"

	MinEffectHeight = 10000
)

type OrderType = byte

var CreateMarketSpendCet sdk.Coin

func init() {
	CreateMarketSpendCet = types.NewCetCoin(CreateMarketFee)
}

func NewHandler(k Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		switch msg := msg.(type) {
		case MsgCreateMarketInfo:
			return handleMsgCreateMarketInfo(ctx, msg, k)
		case MsgCreateOrder:
			return handleMsgCreateOrder(ctx, msg, k)
		case MsgCancelOrder:
			return handleMsgCancelOrder(ctx, msg, k)
		case MsgCancelMarket:
			return handleMsgCancelMarket(ctx, msg, k)
		default:
			errMsg := "Unrecognized market Msg type: %s" + msg.Type()
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

func handleMsgCreateMarketInfo(ctx sdk.Context, msg MsgCreateMarketInfo, keeper Keeper) sdk.Result {
	if ret := checkMsgCreateMarketInfo(ctx, msg, keeper); !ret.IsOK() {
		return ret
	}

	info := MarketInfo{
		Stock:             msg.Stock,
		Money:             msg.Money,
		Creator:           msg.Creator,
		PricePrecision:    msg.PricePrecision,
		LastExecutedPrice: sdk.ZeroDec(),
	}

	key := marketStoreKey(MarketIdentifierPrefix, info.Stock+SymbolSeparator+info.Money)
	value := keeper.cdc.MustMarshalBinaryBare(info)
	ctx.KVStore(keeper.marketKey).Set(key, value)

	return sdk.Result{Tags: info.GetTags()}
}

func checkMsgCreateMarketInfo(ctx sdk.Context, msg MsgCreateMarketInfo, keeper Keeper) sdk.Result {
	key := marketStoreKey(MarketIdentifierPrefix, msg.Stock+SymbolSeparator+msg.Money)
	store := ctx.KVStore(keeper.marketKey)
	if v := store.Get(key); v != nil {
		return ErrInvalidSymbol().Result()
	}

	if !keeper.axk.IsTokenExists(ctx, msg.Money) || !keeper.axk.IsTokenExists(ctx, msg.Stock) {
		return ErrTokenNoExist().Result()
	}

	if !keeper.axk.IsTokenIssuer(ctx, msg.Stock, []byte(msg.Creator)) && !keeper.axk.IsTokenIssuer(ctx, msg.Money, []byte(msg.Creator)) {
		return ErrInvalidTokenIssuer().Result()
	}

	if msg.PricePrecision < MinTokenPricePrecision || msg.PricePrecision > MaxTokenPricePrecision {
		return ErrInvalidPricePrecision().Result()
	}

	if !keeper.bnk.HasCoins(ctx, msg.Creator, sdk.Coins{CreateMarketSpendCet}) {
		return ErrInsufficientCoins().Result()
	}

	return sdk.Result{}
}

func handleMsgCreateOrder(ctx sdk.Context, msg MsgCreateOrder, keeper Keeper) sdk.Result {
	store := ctx.KVStore(keeper.marketKey)
	if store == nil {
		return ErrNoStoreEngine().Result()
	}

	if ret := checkMsgCreateOrder(ctx, store, msg, keeper); !ret.IsOK() {
		return ret
	}

	order := Order{
		Sender:      msg.Sender,
		Sequence:    msg.Sequence,
		Symbol:      msg.Symbol,
		OrderType:   msg.OrderType,
		Price:       sdk.NewDec(msg.Price),
		Quantity:    msg.Quantity,
		Side:        msg.Side,
		TimeInForce: msg.TimeInForce,
		Height:      ctx.BlockHeight(),
		LeftStock:   0,
		Freeze:      0,
		DealMoney:   0,
		DealStock:   0,
	}

	ork := NewOrderKeeper(keeper.marketKey, order.Symbol, keeper.cdc)
	if err := ork.Add(ctx, &order); err != nil {
		return err.Result()
	}
	//TODO. Need Add freeze coin logic
	return sdk.Result{Tags: order.GetTagsInOrderCreate()}
}

func checkMsgCreateOrder(ctx sdk.Context, store sdk.KVStore, msg MsgCreateOrder, keeper Keeper) sdk.Result {
	if err := msg.ValidateBasic(); err != nil {
		return err.Result()
	}

	//acc := authx.NewAccountXWithAddress(msg.Sender)
	//GetAccountSequ
	values := strings.Split(msg.Symbol, SymbolSeparator)
	denom := values[0]
	if msg.Side == match.BUY {
		denom = values[1]
	}

	marketInfo, err := keeper.GetMarketInfo(ctx, msg.Symbol)
	if err != nil || msg.PricePrecision > marketInfo.PricePrecision {
		return ErrInvalidPricePrecision().Result()
	}

	coin := sdk.NewCoin(denom, calculateAmount(msg.Price, msg.Quantity, msg.PricePrecision).RoundInt())
	if !keeper.bnk.HasCoins(ctx, msg.Sender, sdk.Coins{coin}) {
		return ErrInsufficientCoins().Result()
	}

	if keeper.axk.IsTokenFrozen(ctx, denom) {
		return ErrTokenFrozenByIssuer().Result()
	}

	return sdk.Result{}
}

func handleMsgCancelOrder(ctx sdk.Context, msg MsgCancelOrder, keeper Keeper) sdk.Result {
	if err := msg.ValidateBasic(); err != nil {
		return err.Result()
	}

	globalKeeper := NewGlobalOrderKeeper(keeper.marketKey, keeper.cdc)
	order := globalKeeper.QueryOrder(ctx, msg.OrderID)
	if order == nil {
		return sdk.NewError(StoreKey, CodeNotFindOrder, "Not find order in blockchain").Result()
	}

	if !bytes.Equal(order.Sender, msg.Sender) {
		return sdk.NewError(StoreKey, CodeNotMatchSender, "The cancel addr is not match order sender").Result()
	}

	//TODO. Need add unfreeze token logic.
	ork := NewOrderKeeper(keeper.marketKey, order.Symbol, keeper.cdc)
	if err := ork.Remove(ctx, order); err != nil {
		return err.Result()
	}

	return sdk.Result{}
}

func handleMsgCancelMarket(ctx sdk.Context, msg MsgCancelMarket, keeper Keeper) sdk.Result {

	if err := checkMsgCancelMarket(keeper, msg, ctx); err != nil {
		return err.Result()
	}

	dlk := NewDelistKeeper(keeper.marketKey)
	dlk.AddDelistRequest(ctx, msg.EffectiveHeight, msg.Symbol)

	return sdk.Result{}
}

func checkMsgCancelMarket(keeper Keeper, msg MsgCancelMarket, ctx sdk.Context) sdk.Error {

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	currHeight := ctx.BlockHeight()
	if msg.EffectiveHeight < currHeight+MinEffectHeight {
		return sdk.NewError(CodeSpaceMarket, CodeInvalidHeight, "Invalid Height")
	}

	info, err := keeper.GetMarketInfo(ctx, msg.Symbol)
	if err != nil {
		return sdk.NewError(CodeSpaceMarket, CodeInvalidSymbol, err.Error())
	}

	if !bytes.Equal(info.Creator, msg.Sender) {
		return sdk.NewError(CodeSpaceMarket, CodeNotMatchSender, "Not match market info sender")
	}

	return nil
}

func calculateAmount(price, quantity int64, pricePrecision byte) sdk.Dec {
	actualPrice := sdk.NewDec(price).Quo(sdk.NewDec(int64(math.Pow10(int(pricePrecision)))))
	return actualPrice.Mul(sdk.NewDec(quantity))
}

func marketStoreKey(prefix []byte, params ...string) []byte {
	buf := bytes.NewBuffer(prefix)
	for _, param := range params {
		if _, err := buf.Write([]byte(param)); err != nil {
			panic(err)
		}
	}
	return buf.Bytes()
}
