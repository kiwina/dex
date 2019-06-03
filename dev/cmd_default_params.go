package dev

import (
	"fmt"
	"os"
	"reflect"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/params"

	"github.com/coinexchain/dex/modules/asset"
	"github.com/coinexchain/dex/modules/authx"
	"github.com/coinexchain/dex/modules/bankx"
	"github.com/coinexchain/dex/modules/market"
	"github.com/coinexchain/dex/modules/stakingx"
)

type moduleParamSet struct {
	moduleName string
	paramSet   params.ParamSet
}

func DefaultParamsCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default-params",
		Short: "Print default params",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printDefaultParams(getParamSets())
			return nil
		},
	}
	cmd.Flags().Bool("include-sdk", false, "include params defined by cosmos-sdk modules")
	return cmd
}

func getParamSets() []moduleParamSet {
	return []moduleParamSet{
		toParamSet("authx", authx.DefaultParams()),
		toParamSet("bankx", bankx.DefaultParams()),
		toParamSet("stakingx", stakingx.DefaultParams()),
		toParamSet("asset", asset.DefaultParams()),
		toParamSet("market", market.DefaultParams()),
	}
}

func toParamSet(moduleName string, obj interface{}) moduleParamSet {
	vp := reflect.New(reflect.TypeOf(obj))
	vp.Elem().Set(reflect.ValueOf(obj))
	vpi := vp.Interface()
	return moduleParamSet{
		moduleName: moduleName,
		paramSet:   vpi.(params.ParamSet),
	}
}

func printDefaultParams(paramSets []moduleParamSet) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Module", "Key", "Value", "Type"})
	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_LEFT,
	})

	for _, paramSet := range paramSets {
		fillParamsTable(table, paramSet.moduleName, paramSet.paramSet)
	}

	table.Render()
}

func fillParamsTable(table *tablewriter.Table, moduleName string, ps params.ParamSet) {
	for _, pair := range ps.ParamSetPairs() {
		t := reflect.Indirect(reflect.ValueOf(pair.Value)).Type().Name()
		v := reflect.Indirect(reflect.ValueOf(pair.Value)).Interface()
		table.Append([]string{moduleName, string(pair.Key), fmt.Sprintf("%v", v), t})
	}
}
