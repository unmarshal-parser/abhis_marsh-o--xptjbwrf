// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HydroProtocol/ethereum-watcher/blockchain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	sdkTransactionTypes "github.com/eucrypt/unmarshal-go-sdk/pkg/transaction_details/types"
	"github.com/jackc/pgconn"
	"github.com/onrik/ethrpc"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	commonLog "log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	_ = sdkTransactionTypes.RawTransaction{}
	_ = pgconn.PgError{}
	_ = big.NewInt
	_ = ethrpc.Transaction{}
	_ = sync.Mutex{}
	_ = fmt.Sprint()
)

func ToEthLog(txLog blockchain.ReceiptLog) types.Log {
	hashTopics := make([]common.Hash, 0)
	for _, topic := range txLog.GetTopics() {
		hashTopics = append(hashTopics, common.HexToHash(topic))
	}
	return types.Log{
		Address:     common.HexToAddress(txLog.GetAddress()),
		Topics:      hashTopics,
		Data:        common.FromHex(txLog.GetData()),
		BlockNumber: uint64(txLog.GetBlockNum()),
		TxHash:      common.HexToHash(txLog.GetTransactionHash()),
		TxIndex:     uint(txLog.GetTransactionIndex()),
		BlockHash:   common.HexToHash(txLog.GetBlockHash()),
		Index:       uint(txLog.GetLogIndex()),
		Removed:     txLog.GetRemoved(),
	}
}

func NewPostgresOrm(postgresConfig PostgresConfig, autoMigrateTypes ...interface{}) (*gorm.DB, error) {
	customLogger := logger.New(
		commonLog.New(os.Stdout, "\r\n", commonLog.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second,   // Slow SQL threshold
			LogLevel:                  logger.Silent, // Log level
			IgnoreRecordNotFoundError: true,          // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,         // Disable color
		},
	)
	orm, err := gorm.Open(postgres.Open(postgresConfig.ConnectionString), &gorm.Config{
		Logger:          customLogger,
		CreateBatchSize: postgresConfig.CreateBatchSize,
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   postgresConfig.TablePrefix,
			SingularTable: false,
		}})
	if err != nil {
		return nil, err
	}
	err = setMaxDbConnections(orm, postgresConfig.MaxConnections)
	if err != nil {
		return nil, err
	}
	err = orm.AutoMigrate(autoMigrateTypes...)
	if err != nil {
		return nil, err
	}
	return orm, nil
}

func setMaxDbConnections(orm *gorm.DB, maxConnections int) error {
	psql, err := orm.DB()
	if err != nil {
		return err
	}
	psql.SetMaxIdleConns(5)
	psql.SetMaxOpenConns(maxConnections)
	return nil
}

type ApprovalEventTracker struct {
	Orm          *gorm.DB
	MainFilterer *MainFilterer
	chainID      string
}

func NewApprovalEventTracker(contractAddress string, orm *gorm.DB, ethClient *ethclient.Client, chainID string) (ApprovalEventTracker, error) {
	err := orm.AutoMigrate(&ApprovalEvent{})
	if err != nil {
		return ApprovalEventTracker{}, err
	}
	filterer, err := NewMainFilterer(common.HexToAddress(contractAddress), ethClient)
	return ApprovalEventTracker{
		Orm:          orm,
		MainFilterer: filterer,
		chainID:      chainID,
	}, err
}

func (a ApprovalEventTracker) CreateApproval(txLog blockchain.ReceiptLog, ethClient *EthBlockChainRPCWithRetry, u *UnmarshalSDKWrapper, txDetailsMap map[string]sdkTransactionTypes.TxnByID) (ApprovalEvent, error) {
	ethLog := ToEthLog(txLog)
	abiEvent, err := a.MainFilterer.ParseABIApprovalEvent(ethLog, *txLog.Log)
	if err != nil {
		return ApprovalEvent{}, err
	}
	var transactionData sdkTransactionTypes.TxnByID
	transactionData, present := txDetailsMap[ethLog.TxHash.String()]
	if !present {
		log.WithFields(log.Fields{"err": "TransactionDetailsMissing", "transactionHash": ethLog.TxHash.String()}).Info("")
		transactionData, err = u.GetTransactionByHash(ethLog.TxHash.String())
	}
	if err != nil {
		blockDetail, err := ethClient.GetBlockByNumWithoutDetails(txLog.GetBlockNum())
		if err != nil {
			return ApprovalEvent{}, err
		}
		nodeTxnData, err := ethClient.GetTxByHash(txLog.TransactionHash)
		if err != nil {
			return ApprovalEvent{}, err
		}
		transactionData.Fee = fmt.Sprintf("%d", nodeTxnData.Gas)
		transactionData.GasPrice = nodeTxnData.GasPrice.String()
		transactionData.From = nodeTxnData.From
		transactionData.To = nodeTxnData.To
		transactionData.Value = nodeTxnData.Value.String()
		transactionData.Date = blockDetail.Timestamp
	}
	event := convertToApprovalEvent(abiEvent, transactionData, a.chainID)
	return event, nil
}

func EventIndexCallback(orm *gorm.DB, u *UnmarshalSDKWrapper, approvalEventTracker ApprovalEventTracker, txLogs []ethrpc.Log, ethClient *EthBlockChainRPCWithRetry) error {
	errs, _ := errgroup.WithContext(context.Background())

	approvalEventsLock := new(sync.Mutex)
	approvalEvents := make([]ApprovalEvent, 0)

	transactionDetailsMap := getBulkTransactionDetails(txLogs, u)
	for i := range txLogs {
		i := i

		errs.Go(func() error {
			return func(i int) error {
				txLog := blockchain.IReceiptLog(blockchain.ReceiptLog{Log: &txLogs[i]})
				switch txLog.GetTopics()[0] {

				case GetApprovalEventHash():
					event, err := approvalEventTracker.CreateApproval(txLog.(blockchain.ReceiptLog), ethClient, u, transactionDetailsMap)
					if err != nil {
						log.WithFields(log.Fields{"txHash": txLog.GetTransactionHash(), "blockNumber": txLog.GetBlockNum(), "err": err}).
							Error("ApprovalEvent: Failed to create event from log")
						return err
					}

					err = event.BeforeCreateHook(orm)
					if err != nil {
						log.WithField("err", err).Error("Error making pre create hook call")
					}
					approvalEventsLock.Lock()
					approvalEvents = append(approvalEvents, event)
					approvalEventsLock.Unlock()

				default:
					return nil
				}
				return nil
			}(i)
		})

	}
	err := errs.Wait()
	if err != nil {
		return err
	}

	wg := new(sync.WaitGroup)

	err = orm.Clauses(clause.OnConflict{DoNothing: true}).Create(&approvalEvents).Error
	if err != nil {
		return err
	}
	for _, event := range approvalEvents {
		wg.Add(1)
		go func(routineEvent ApprovalEvent, wg *sync.WaitGroup) {
			defer wg.Done()
			err = routineEvent.AfterCreateHook(orm)
			if err != nil {
				log.WithFields(log.Fields{"err": err, "Entity": "Approval"}).Error("EventIndexCallback: Error making post create hook call")
			}
		}(event, wg)
	}

	wg.Wait()
	return nil
}

func getBulkTransactionDetails(txLogs []ethrpc.Log, u *UnmarshalSDKWrapper) map[string]sdkTransactionTypes.TxnByID {
	var transactionDetails []sdkTransactionTypes.TxnByID
	var transactionHashes []string

	for i, txLog := range txLogs {

		transactionHashes = append(transactionHashes, txLog.TransactionHash)

		if (len(transactionHashes)%100 == 0 || i+1 == len(txLogs)) && len(transactionHashes) > 0 {
			transactionsDetails, err := u.GetBulkTransactionDetailsByHash(transactionHashes)
			transactionHashes = []string{}
			if err != nil {
				log.WithFields(log.Fields{"TransactionRange": txLog.TransactionHash, "err": err}).Error("Bulk transaction fetch failed")
			}
			transactionDetails = append(transactionDetails, transactionsDetails...)
		}
	}

	transactionDetailsMap := make(map[string]sdkTransactionTypes.TxnByID)
	for _, txDetails := range transactionDetails {
		transactionDetailsMap[txDetails.Id] = txDetails
	}

	return transactionDetailsMap
}

func MethodIndexerCallback(orm *gorm.DB, from, to int, contractAddress string, unmarshalSDKWrapper *UnmarshalSDKWrapper) error {
	errs, _ := errgroup.WithContext(context.Background())

	transactionList := fetchTransactions(unmarshalSDKWrapper, from, to, contractAddress)
	if transactionList == nil {
		return nil
	}
	if transactionList == nil {
		return nil
	}
	for j := range transactionList {
		j := j
		errs.Go(func() error {
			return func(j int) error {
				tx := transactionList[j]
				if tx.AdditionalData.Status != 1 {
					return nil
				}
				if tx.To == "" {
					return nil
				}
				if strings.ToLower(tx.To) != strings.ToLower(contractAddress) {
					return nil
				}
				if len(tx.AdditionalData.Data) < 10 {
					return nil
				}
				switch methodHex := tx.AdditionalData.Data[2:10]; methodHex {

				default:
					return nil
				}
				return nil
			}(j)
		})
	}

	err := errs.Wait()
	if err != nil {
		return err
	}

	wg := new(sync.WaitGroup)

	wg.Wait()
	return nil
}

func fetchTransactions(unmarshalSDKWrapper *UnmarshalSDKWrapper, from int, to int, contractAddress string) (transactionList []sdkTransactionTypes.RawTransaction) {
	var err error
	for true {
		transactionList, err = unmarshalSDKWrapper.GetAllTransactionsBetween(from, to, contractAddress)
		if err == nil {
			return
		}
		log.WithFields(log.Fields{"start block": from, "to block": to, "err": err}).
			Error("Failed to fetch transactions for a block")
		time.Sleep(5 * time.Second)
	}
	return
}

type EthBlockChainRPCWithRetry struct {
	client        *ethrpc.EthRPC
	maxRetryTimes int
	blockMetrics  *Metrics
}

func NewEthRPCWithRetry(api string, maxRetryCount int, metrics *Metrics) *EthBlockChainRPCWithRetry {
	rpc := ethrpc.New(api)
	return &EthBlockChainRPCWithRetry{rpc, maxRetryCount, metrics}
}

func (rpc EthBlockChainRPCWithRetry) incrementNodeRestartCounter(location string, err error) {
	if err == nil {
		err = errors.New("forced increment")
	}
	rpc.blockMetrics.NodeRestarts.With(
		prometheus.Labels{
			"location":      location,
			"node_endpoint": rpc.client.URL(),
			"error":         err.Error(),
		},
	).Inc()
}

func (rpc EthBlockChainRPCWithRetry) observeLatency(start time.Time, location string) {
	rpc.blockMetrics.NodeLatency.With(prometheus.Labels{
		"node_endpoint": rpc.client.URL(),
		"method":        location,
	}).Observe(float64(time.Since(start)))
}

func (rpc EthBlockChainRPCWithRetry) GetTopBlock() (resp int, err error) {
	defer rpc.observeLatency(time.Now(), "GetTopBlock")
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		resp, err = rpc.client.EthBlockNumber()
		if err == nil && resp != 0 {
			return
		}
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}
	rpc.incrementNodeRestartCounter("GetTopBlock", err)
	return
}

func (rpc EthBlockChainRPCWithRetry) FetchLogs(contractAddress string, from int, to int, topicsInterested []string) (resp []ethrpc.Log, err error) {
	defer rpc.observeLatency(time.Now(), "FetchLogs")
	filterParam := ethrpc.FilterParams{
		FromBlock: "0x" + strconv.FormatUint(uint64(from), 16),
		ToBlock:   "0x" + strconv.FormatUint(uint64(to), 16),

		Address: []string{contractAddress},

		Topics: [][]string{topicsInterested},
	}
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		resp, err = rpc.client.EthGetLogs(filterParam)
		if err != nil {
			log.WithField("Error", err).Error("Error while fetching logs")
		}
		if err == nil && resp != nil {
			return
		}
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}
	rpc.incrementNodeRestartCounter("FetchLogs", err)
	return
}

func (rpc EthBlockChainRPCWithRetry) getChainID() (chainID string, err error) {
	defer rpc.observeLatency(time.Now(), "getChainID")
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		var hexChainID string
		var chainIDInt uint64
		err = rpc.makeEthCallAndSave("eth_chainId", &hexChainID)
		if err != nil {
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
			continue
		}
		chainIDInt, err = strconv.ParseUint(hexChainID, 0, 0)
		if err != nil {
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
			continue
		}
		chainID = strconv.FormatUint(chainIDInt, 10)
		log.Info("Chain ID: ", chainID)
		return
	}
	rpc.incrementNodeRestartCounter("getChainID", err)
	return
}

func (rpc EthBlockChainRPCWithRetry) GetBlockByNumWithoutDetails(num int) (resp *ethrpc.Block, err error) {
	defer rpc.observeLatency(time.Now(), "GetBlockByNumWithoutDetails")
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		resp, err = rpc.client.EthGetBlockByNumber(num, false)
		if err == nil && isNotEmptyBlockDetails(resp) {
			return
		}
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}
	rpc.incrementNodeRestartCounter("GetBlockByNumWithoutDetails", err)
	return
}

func isNotEmptyBlockDetails(block *ethrpc.Block) bool {
	if block == nil {
		return false
	}
	if block.Number == 0 && block.Timestamp == 0 && block.Hash == "" {
		return false
	}
	return true
}

func (rpc EthBlockChainRPCWithRetry) GetTxByHash(txHash string) (resp *ethrpc.Transaction, err error) {
	defer rpc.observeLatency(time.Now(), "GetTxByHash")
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		resp, err = rpc.client.EthGetTransactionByHash(txHash)
		if err == nil && isNotEmptyTxDetails(resp) {
			return
		}
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}
	rpc.incrementNodeRestartCounter("GetTxByHash", err)
	return
}

func isNotEmptyTxDetails(tx *ethrpc.Transaction) bool {
	if tx == nil {
		return false
	}
	if tx.BlockNumber == nil && tx.TransactionIndex == nil && tx.Hash == "" {
		return false
	}
	return true
}

func (rpc EthBlockChainRPCWithRetry) makeEthCallAndSave(methodName string, result interface{}) (err error) {
	var resp json.RawMessage
	for i := 0; i <= rpc.maxRetryTimes; i++ {
		resp, err = rpc.client.Call(methodName)
		if err != nil || resp == nil {
			log.WithField("Method Name", methodName).Error("RPC Call failed")
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
			continue
		}
		err = json.Unmarshal(resp, result)
		if err != nil {
			log.WithField("Method Name", methodName).Error("Failed to unmarshal response")
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
			continue
		}
		break
	}
	return
}

type SyncDB struct {
	orm *gorm.DB

	contractAddress string
	chainID         string
	syncType        string
}

func InitSyncDB(orm *gorm.DB) error {
	return orm.AutoMigrate(&LastSyncedBlock{})
}

func NewSyncDB(orm *gorm.DB, contract, chainID string, syncType string) SyncDB {
	return SyncDB{orm: orm, contractAddress: contract, chainID: chainID, syncType: syncType}
}

func (sync SyncDB) UpdateLastSynced(blockNum int) error {
	res := sync.orm.Save(&LastSyncedBlock{
		Contract:    sync.contractAddress,
		ChainID:     sync.chainID,
		BlockNumber: uint64(blockNum),
		SyncType:    sync.syncType,
	})
	if res.Error == nil {
		log.Infof("LastSyncedBlock: success, SyncType: %s , BlockNumber: %d", sync.syncType, blockNum)
	}
	return res.Error
}

func (sync SyncDB) GetLastSyncedBlock() (int, error) {
	lastSync := &LastSyncedBlock{Contract: sync.contractAddress}
	res := sync.orm.First(lastSync, "contract = ? and chain_id = ? and sync_type = ?", sync.contractAddress, sync.chainID, sync.syncType)
	return int(lastSync.BlockNumber), res.Error
}

func RunIndexer(config IndexerConfig) {
	initUnmarshalSDK(config)
	topicsInterestedIn := []string{
		GetApprovalEventHash(),
	}
	methodsInterestedIn := []string{}
	orm, err := NewPostgresOrm(config.PostgresConfig)
	if err != nil {
		panic(err)
	}
	err = InitSyncDB(orm)
	if err != nil {
		panic(err)
	}
	ethClient := new(ethclient.Client)
	ethRPCClient := NewEthRPCWithRetry(config.EthEndpoint, 3, indexerConfig.Metrics)
	chainID, err := ethRPCClient.getChainID()
	if err != nil || chainID == "" {
		log.WithField("Error", err).Error("Failed to get chain id, Shutting down.")
		return
	}
	chain, err := GetChainFromChainID(chainID)
	if err != nil || chainID == "" {
		log.Error("Chain not supported by SDK")
		panic(err)
	}

	err = InitPluginModels(orm)
	if err != nil {
		panic(err)
	}

	var unmarshalSDKWrapper = NewUnmarshalSDKWrapper(&sdk, chain, 4, indexerConfig.Metrics)
	receiptsChannel := make(chan Receipts, config.ParallelCalls)
	wg := new(sync.WaitGroup)

	//syncing events
	if len(topicsInterestedIn) > 0 {
		wg.Add(2)
		eventSyncDB := NewSyncDB(orm, config.ContractAddress, chainID, "events")
		fetchAndPushLogsToChannel(eventSyncDB, config, ethRPCClient, topicsInterestedIn, receiptsChannel, wg)
		processLogsFromChannel(orm, eventSyncDB, config.ParallelCalls, chainID, config, ethClient, receiptsChannel, ethRPCClient, unmarshalSDKWrapper, wg)
	}

	//syncing methods
	if len(methodsInterestedIn) > 0 {
		wg.Add(1)
		syncDB := NewSyncDB(orm, config.ContractAddress, chainID, "methods")
		syncMethods(orm, syncDB, ethRPCClient, config, ethClient, chainID, unmarshalSDKWrapper)
	}
	wg.Wait()
}

func fetchAndPushLogsToChannel(syncDB SyncDB, config IndexerConfig, rpcClient *EthBlockChainRPCWithRetry, topicsInterestedIn []string, receiptsChannel chan Receipts, wg *sync.WaitGroup) {

	go func() {
		log.Infof("Started go routine to fetch logs")
		startBlock := config.StartBlock
		lastUpdatedBlock, err := syncDB.GetLastSyncedBlock()
		if err == nil {
			if lastUpdatedBlock > startBlock {
				startBlock = lastUpdatedBlock + 1
			}
		}
		log.Infof("StartBlock ----> %d\n", startBlock)
		parallelCallsForLogs := config.ParallelCalls
		stepSize := config.StepSize
		mutex := sync.Mutex{}
		for true {
			errs, _ := errgroup.WithContext(context.Background())
			receiptsMap := map[int]Receipts{}
			highestBlockNumber, e := rpcClient.GetTopBlock()
			if e != nil {
				log.WithField("Error", e).Error("Failed to get node top block")
				continue
			}
			highestBlockNumber -= config.LagToHighestBlock
			if isParserInLiveSync(highestBlockNumber, startBlock, parallelCallsForLogs, stepSize) {
				log.Infof("Not enough blocks, waiting for new blocks , parallelCallsForLogs=%d, stepSize=%d", parallelCallsForLogs, stepSize)
				parallelCallsForLogs--
				stepSize = 1
				if parallelCallsForLogs >= 1 {
					continue
				}
				parallelCallsForLogs = 1
				log.Info("Parser in Live Sync. Max Lag reached. Sleeping...")
				time.Sleep(3 * time.Second)
				continue
			}
			for i := 0; i < parallelCallsForLogs; i++ {
				i := i
				start := startBlock
				errs.Go(func() error {
					return func(i int, start int) error {
						rec, err := rpcClient.FetchLogs(config.ContractAddress, start+(i*stepSize), start+(i*stepSize)+stepSize-1, topicsInterestedIn)
						if err != nil {
							return err
						}
						fetchedReceipts := Receipts{
							receipts: rec,
							i:        i,
							from:     start + (i * stepSize),
							to:       start + (i * stepSize) + stepSize - 1,
						}
						mutex.Lock()
						receiptsMap[i] = fetchedReceipts
						mutex.Unlock()
						return nil
					}(i, start)
					return nil
				})
			}
			err := errs.Wait()
			if err != nil {
				log.WithField("Error", err).Error("fetchAndPushLogsToChannel")
				continue
			}
			startBlock = startBlock + (parallelCallsForLogs * stepSize)
			log.WithField("Last Block Processed", receiptsMap[parallelCallsForLogs-1].to).
				Info("Fetched Logs in Batch")
			for i := 0; i < parallelCallsForLogs; i++ {
				receiptsChannel <- receiptsMap[i]
			}
		}
		wg.Done()
	}()
}

func isParserInLiveSync(highestBlockNumber int, startBlock int, parallelCallsForLogs int, stepSize int) bool {
	return highestBlockNumber-(startBlock+(parallelCallsForLogs*stepSize)) < 0
}

func processLogsFromChannel(orm *gorm.DB, syncDB SyncDB, parallelCallForDB int, chainID string, config IndexerConfig, ethClient *ethclient.Client, receiptsChannel chan Receipts, ethRPCClient *EthBlockChainRPCWithRetry, unmarshalSDKWrapper *UnmarshalSDKWrapper, wg *sync.WaitGroup) {

	approvalEventTracker, err := NewApprovalEventTracker(config.ContractAddress, orm, ethClient, chainID)
	if err != nil {
		panic(err)
	}

	go func() {

		log.Info("Starting go routine for processing logs")
		for true {
			var loopWaitGroup sync.WaitGroup
			var lastUpdatedReceipt Receipts
			for i := 0; i < parallelCallForDB; i++ {
				loopWaitGroup.Add(1)
				lastUpdatedReceipt = <-receiptsChannel
				go func(routineReceiptLog Receipts) {
					defer loopWaitGroup.Done()

					log.WithFields(log.Fields{
						"From":           routineReceiptLog.from,
						"To":             routineReceiptLog.to,
						"Number of logs": len(routineReceiptLog.receipts)}).
						Info("Processing logs")

					err := EventIndexCallback(orm, unmarshalSDKWrapper, approvalEventTracker, routineReceiptLog.receipts, ethRPCClient)
					if err != nil {
						log.WithField("Location:", "processLogsFromChannel").
							Error("Error with EventIndexCallBack")
						panic(err)
					}
				}(lastUpdatedReceipt)
			}
			loopWaitGroup.Wait()
			err := syncDB.UpdateLastSynced(lastUpdatedReceipt.to)
			if err != nil {
				log.WithField("Location:", "processLogsFromChannel").
					Error("Error updating last synced block")
				panic(err)
			}
			log.WithFields(log.Fields{"From": lastUpdatedReceipt.from, "To": lastUpdatedReceipt.to}).Info("Finished Processing logs")
		}
		wg.Done()
	}()
}

func syncMethods(orm *gorm.DB, syncDB SyncDB, rpcClient *EthBlockChainRPCWithRetry, config IndexerConfig, ethClient *ethclient.Client, chainID string, unmarshalSDKWrapper *UnmarshalSDKWrapper) {
	stepSize := config.StepSize
	startBlock := config.StartBlock
	parallelCalls := config.ParallelCalls
	lastUpdatedBlock, err := syncDB.GetLastSyncedBlock()
	if err == nil {
		if lastUpdatedBlock > startBlock {
			startBlock = lastUpdatedBlock + 1
		}
	}

	go func() {
		log.WithField("Start Block", startBlock).Info("Syncing Methods")
		for true {
			highestBlockNumber, err := rpcClient.GetTopBlock()
			if err != nil {
				log.WithFields(log.Fields{"Location": "Sync Methods", "Error": err}).Error("Failed to get Top Block")
				continue
			}
			highestBlockNumber -= config.LagToHighestBlock
			if isParserInLiveSync(highestBlockNumber, startBlock, parallelCalls, stepSize) {
				log.WithFields(log.Fields{"Location": "syncMethods", "Step Size": stepSize}).
					Info("Not enough blocks, waiting for new blocks")
				time.Sleep(3 * time.Second)
				if parallelCalls == 1 {
					if stepSize > 1 {
						stepSize--
						log.WithFields(log.Fields{"Location": "syncMethods", "Step Size": stepSize}).
							Info("Updated Step Size")
					}
					continue
				}
				parallelCalls--
				log.WithFields(log.Fields{"Location": "syncMethods", "parallel Calls": parallelCalls}).
					Info("Updated number of parallel calls")
				continue
			}
			from := startBlock
			to := startBlock + stepSize
			functionWaitGroup := new(sync.WaitGroup)
			for i := 0; i < parallelCalls; i++ {
				functionWaitGroup.Add(1)
				go func(routineFrom, routineTo int) {
					defer functionWaitGroup.Done()
					log.WithFields(log.Fields{"From": routineFrom, "To": routineTo}).Info("Fetching transactions")
					err = MethodIndexerCallback(orm, routineFrom, routineTo, config.ContractAddress, unmarshalSDKWrapper)
					if err != nil {
						log.Error("Error with MethodIndexerCallback:")
						panic(err)
					}
					return
				}(from, to)
				from = to
				to += stepSize
			}
			functionWaitGroup.Wait()
			if err != nil {
				log.Error("Error with MethodIndexerCallback:")
				panic(err)
			}
			err = syncDB.UpdateLastSynced(from)
			if err != nil {
				log.WithField("Location", "syncMethods").Error("Failed to update last synced: ", err)
				panic(err)
			}
			log.WithFields(log.Fields{"From": startBlock, "To": from}).Info("Finished processing Method Batch")
			startBlock = to
		}
	}()
}

type Receipts struct {
	from     int
	to       int
	i        int
	receipts []ethrpc.Log
	pairMap  map[string]bool
}

func LoadConfig(file string, path string, cfg *IndexerConfig) (err error) {
	viper.SetConfigName(file)
	viper.AddConfigPath(path)
	viper.AutomaticEnv()
	viper.SetConfigType("yml")
	err = viper.ReadInConfig()
	if err != nil {
		log.Errorf("error reading config file %s", err)
		return
	}
	log.Println("Config loaded successfully...")
	log.Println("Getting environment variables...")
	for _, k := range viper.AllKeys() {
		value := viper.GetString(k)
		if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
			viper.Set(k, getEnvOrPanic(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")))
		}
	}
	err = viper.Unmarshal(cfg)
	if err != nil {
		log.Errorf("unable to decode config to struct, %v", err)
		return
	}
	return
}

func getEnvOrPanic(env string) string {
	res := os.Getenv(env)
	if len(res) == 0 {
		panic("Mandatory env variable not found:" + env)
	}
	return res
}
