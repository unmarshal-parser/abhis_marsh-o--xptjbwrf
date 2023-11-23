// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"fmt"
	unmarshal "github.com/eucrypt/unmarshal-go-sdk/pkg"
	conf "github.com/eucrypt/unmarshal-go-sdk/pkg/config"
	"github.com/eucrypt/unmarshal-go-sdk/pkg/constants"
	sdkTokenDetails "github.com/eucrypt/unmarshal-go-sdk/pkg/token_details"
	"github.com/eucrypt/unmarshal-go-sdk/pkg/token_details/types"
	tokenPriceTypes "github.com/eucrypt/unmarshal-go-sdk/pkg/token_price/types"
	"github.com/onrik/ethrpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var (
	_   = types.TokenDetails{}
	_   = prometheus.Labels{}
	_   = decimal.Decimal{}
	_   = big.NewInt
	_   = ethrpc.Transaction{}
	sdk = unmarshal.Unmarshal{}
	_   = sdkTokenDetails.TokenDetailsOptions{}
	_   = gorm.Model{}
	_   = time.Time{}
	_   = strconv.NumError{}
	_   = math.NaN()
	_   = log.Error
	_   = tokenPriceTypes.TokenPrice{}
	_   = fmt.Sprint()
)

const (
	defaultDecimals = 18
)

func InitPluginModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&TokenDetails{},
	)
	if err != nil {
		return err
	}
	return nil
}

func initUnmarshalSDK(cfg IndexerConfig) {
	key := strings.TrimSpace(cfg.ApiKey)
	if key == "" {
		panic("missing api key")
	}
	sdk = unmarshal.NewWithConfig(conf.Config{
		AuthKey:     key,
		Environment: constants.Prod,
	})
}

func (entity *ApprovalEvent) BeforeCreateHook(tx *gorm.DB) error {

	tokenDetailsEventValue, err := getTokenDetails("0x5a666c7d92e5fa7edcb6390e4efd6d0cdd69cf37", tx, entity.ChainID)
	if err != nil {
		tokenDetailsEventValue.Decimal = defaultDecimals
	}
	entity.DecimalAdjustedEventValue = formatAmount(entity.EventValue, tokenDetailsEventValue.Decimal)
	tokenPriceEventValue := getPriceAtInstant("0x5a666c7d92e5fa7edcb6390e4efd6d0cdd69cf37", tokenDetailsEventValue.Symbol, entity.ChainID, entity.BlockTime)
	entity.TokenPriceEventValue = mustParseFloat(tokenPriceEventValue)

	return nil
}

func (entity *ApprovalEvent) AfterCreateHook(tx *gorm.DB) error {
	return nil
}

func formatAmount(amountWithoutDecimal decimal.Decimal, decimals int) float64 {
	value := mustParseFloat(amountWithoutDecimal.String())
	divider := math.Pow(10, float64(decimals))
	value = value / divider
	return value
}

func mustParseFloat(floatVal string) float64 {
	value, err := strconv.ParseFloat(floatVal, 64)
	if err != nil {
		return 0.0
	}
	return value
}

func getPriceAtInstant(contractAddress, tokenSymbol string, indexerChainId string, timestamp time.Time) string {
	chainName, _ := GetChainFromChainID(indexerChainId)
	if IsPriceSupportedForChain(chainName) {
		log.WithFields(log.Fields{
			"chain":   chainName,
			"address": contractAddress,
		}).Debug("Attempting fetch price by contract address")
		price, err := getPriceByContractAddress(contractAddress, chainName, timestamp)
		if err == nil {
			return price.Price
		}
	}

	log.WithFields(log.Fields{
		"chain":        chainName,
		"token_symbol": tokenSymbol,
	}).Debug("Attempting fetch price by symbol")
	priceDetails, err := getPriceBySymbol(tokenSymbol, timestamp)
	if err != nil || len(priceDetails) == 0 {
		log.WithFields(log.Fields{"token": tokenSymbol, "time": time.Now().Unix(), "Contract Address": contractAddress}).Error("Price not found")
		return "0"
	}
	if err == nil {
		for _, priceDetail := range priceDetails {
			if strings.ToLower(priceDetail.Blockchain) == strings.ToLower(chainName.String()) {
				return priceDetail.Price
			}
		}
	}
	return priceDetails[0].Price
}

func getPriceByContractAddress(contractAddress string, chainName constants.Chain, timestamp time.Time) (tokenPriceTypes.TokenPrice, error) {
	defer observePricestoreLatency(time.Now(), "GetTokenPrice", contractAddress, timestamp)
	price, err := sdk.PriceStore.GetTokenPrice(chainName, contractAddress, &tokenPriceTypes.GetPriceOptions{
		Timestamp: uint64(timestamp.Unix()),
	})
	if err != nil {
		incrementPricestoreFailure("GetTokenPrice", contractAddress, timestamp)
	}
	return price, err
}

func getPriceBySymbol(tokenSymbol string, timestamp time.Time) (tokenPriceTypes.PriceWithSymbolResp, error) {
	defer observePricestoreLatency(time.Now(), "GetTokenPriceBySymbol", tokenSymbol, timestamp)
	priceDetails, err := sdk.PriceStore.GetTokenPriceBySymbol(tokenSymbol, &tokenPriceTypes.GetPriceWithSymbolOptions{
		Timestamp: uint64(timestamp.Unix()),
	})
	if err != nil {
		incrementPricestoreFailure("GetTokenPrice", tokenSymbol, timestamp)
	}
	return priceDetails, err
}

func getTokenDetails(tokenAddress string, db *gorm.DB, chainID string) (TokenDetails, error) {
	tokenDetails, ok := getTokenDetailsFromCache(tokenAddress, chainID)
	if ok {
		return tokenDetails, nil
	}

	details, err := getTokenDetailsFromDbAndUpdateCache(tokenAddress, db, chainID)
	if err == nil && details.Symbol != "" {
		return details, nil
	}

	tokenDetails, err = getTokenDetailsFromTokenStore(tokenAddress, db, chainID)
	if err != nil {
		return tokenDetails, err
	}

	updateTokenCache(tokenAddress, chainID, tokenDetails)
	return tokenDetails, nil
}

func getTokenDetailsFromTokenStore(tokenAddress string, db *gorm.DB, chainID string) (TokenDetails, error) {
	var tokenDetails TokenDetails
	chain, _ := GetChainFromChainID(chainID)
	fetchedTokenDetails, err := getFromTokenStore(tokenAddress, chain)
	if err != nil {
		return TokenDetails{}, err
	}
	tokenDetails = TokenDetails{
		Address: strings.ToLower(tokenAddress),
		Symbol:  fetchedTokenDetails.Symbol,
		Decimal: fetchedTokenDetails.Decimal,
		ChainID: chainID,
		Name:    fetchedTokenDetails.Name,
	}
	err = db.Create(&tokenDetails).Error
	if err != nil {
		log.WithFields(log.Fields{"token": tokenAddress, "error": err}).Error("Token details population error")
	}
	return tokenDetails, nil
}

func getFromTokenStore(tokenAddress string, chain constants.Chain) (fetchedTokenDetails types.TokenDetails, err error) {
	defer observeTokenstoreLatency(time.Now(), "GetTokenDetailsByContract", tokenAddress, chain)
	for i := 0; i < 10; i++ {
		fetchedTokenDetails, err = sdk.TokenDetails.GetTokenDetailsByContract(tokenAddress, &sdkTokenDetails.TokenDetailsOptions{chain})
		if err == nil {
			return
		}
		incrementTokenstoreFailure("GetTokenDetailsByContract", tokenAddress, chain)
		log.WithFields(log.Fields{"token": tokenAddress}).Error("Details not found")
		time.Sleep(3 * time.Second)
	}
	return
}

func getTokenDetailsFromDbAndUpdateCache(tokenAddress string, db *gorm.DB, chainID string) (TokenDetails, error) {
	var tokenDetails TokenDetails
	err := db.Where("address = ? and chain_id = ?", strings.ToLower(tokenAddress), chainID).First(&tokenDetails).Error
	if err != nil {
		return TokenDetails{}, err
	}
	updateTokenCache(tokenAddress, chainID, tokenDetails)
	return tokenDetails, nil
}

func getTokenDetailsFromCache(tokenAddress string, chainID string) (TokenDetails, bool) {
	tokenDetails, ok := tokenCache.Load(getChainTokenAddressKey(tokenAddress, chainID))
	if ok {
		return tokenDetails.(TokenDetails), ok
	}
	return TokenDetails{}, false
}

func getChainTokenAddressKey(tokenAddress string, chainID string) string {
	return strings.ToLower(chainID + "_" + tokenAddress)
}

func getWrappedTokenContractAddress(chainIDStr string) (contractAddress string) {
	chain, _ := GetChainFromChainID(chainIDStr)
	return WrappedTokenOnChainMap[chain]
}

func updateTokenCache(tokenAddress, chainID string, tokenDetails TokenDetails) (didUpdate bool) {
	if tokenDetails.Address == "" || tokenDetails.Symbol == "" {
		return
	}
	tokenCache.Store(getChainTokenAddressKey(tokenAddress, chainID), tokenDetails)
	return true
}

func observeTokenstoreLatency(start time.Time, api, contractAddress string, chain constants.Chain) {
	indexerConfig.Metrics.TokenStoreLatency.With(prometheus.Labels{
		"api":              api,
		"contract_address": contractAddress,
		"chain":            chain.String(),
	}).Observe(float64(time.Since(start)))
}

func incrementTokenstoreFailure(api, contractAddress string, chain constants.Chain) {
	indexerConfig.Metrics.TokenStoreFailure.With(
		prometheus.Labels{
			"api":              api,
			"contract_address": contractAddress,
			"chain":            chain.String(),
		},
	).Inc()
}

func observePricestoreLatency(start time.Time, api, dataPassed string, timestampFetchedFor time.Time) {
	indexerConfig.Metrics.PriceStoreLatency.With(prometheus.Labels{
		"api":                         api,
		"data_passed":                 dataPassed,
		"timestamp_price_fetched_for": timestampFetchedFor.String(),
	}).Observe(float64(time.Since(start)))
}

func incrementPricestoreFailure(api, dataPassed string, timestampFetchedFor time.Time) {
	indexerConfig.Metrics.PriceStoreFailure.With(
		prometheus.Labels{
			"api":                         api,
			"data_passed":                 dataPassed,
			"timestamp_price_fetched_for": timestampFetchedFor.String(),
		},
	).Inc()
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}
