package tronWallet

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fcwrsmall/tron-wallet/enums"
	"github.com/fcwrsmall/tron-wallet/grpcClient"
	"github.com/fcwrsmall/tron-wallet/grpcClient/proto/api"
	"github.com/fcwrsmall/tron-wallet/grpcClient/proto/core"
	"github.com/fcwrsmall/tron-wallet/util"
	"github.com/golang/protobuf/proto"
)

type Crawler struct {
	Node      enums.Node
	Addresses []string
}

type CrawlResult struct {
	Address      string
	Transactions []CrawlTransaction
}

type CrawlTransaction struct {
	TxId            string
	Confirmations   int64
	FromAddress     string
	ToAddress       string
	Amount          int64
	Symbol          string
	ContractAddress string
}

func (c *Crawler) ScanBlocks(count int) ([]CrawlResult, error) {

	var wg sync.WaitGroup

	var allTransactions [][]CrawlTransaction

	client, err := grpcClient.GetGrpcClient(c.Node)
	if err != nil {
		return nil, err
	}

	block, err := client.GetNowBlock()
	if err != nil {
		return nil, err
	}

	// check block for transaction
	allTransactions = append(allTransactions, c.extractOurTransactionsFromBlock(block))
	if err != nil {
		return nil, err
	}
	blockNumber := block.BlockHeader.RawData.Number

	var errList []error
	for i := count; i > 0; i-- {
		// sleep to avoid 503 error
		time.Sleep(100 * time.Millisecond)
		blockNumber = blockNumber - 1
		wg.Add(1)
		go func() {
			err_ := c.getBlockData(&wg, client, &allTransactions, blockNumber)
			errList = append(errList, err_)
		}()
	}
	wg.Wait()
	for _,err := range errList{
		if err != nil{
			return nil, err
		}
	}
	return c.prepareCrawlResultFromTransactions(allTransactions), nil
}

func (c *Crawler) ScanBlocksFromTo(client *grpcClient.GrpcClient, from int, to int) ([]CrawlResult, error) {

	if to-from < 1 {
		return nil, errors.New("to number should be more than from number")
	}

	var wg sync.WaitGroup

	var allTransactions [][]CrawlTransaction

	for i := to; i > from; i-- {
		// sleep to avoid 503 error
		wg.Add(1)
		time.Sleep(100 * time.Millisecond)
		go c.getBlockData(&wg, client, &allTransactions, int64(i))
	}

	wg.Wait()

	return c.prepareCrawlResultFromTransactions(allTransactions), nil
}

// ==================== private ==================== //

func (c *Crawler) getBlockData(wg *sync.WaitGroup, client *grpcClient.GrpcClient, allTransactions *[][]CrawlTransaction, num int64)error {

	defer wg.Done()

	block, err := client.GetBlockByNum(num)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// check block for transaction
	*allTransactions = append(*allTransactions, c.extractOurTransactionsFromBlock(block))
	return nil
}

func (c *Crawler) extractOurTransactionsFromBlock(block *api.BlockExtention) []CrawlTransaction {

	var txs []CrawlTransaction

	for _, t := range block.Transactions {

		transaction := t.Transaction

		// if transaction is not success
		if transaction.Ret[0].ContractRet != core.Transaction_Result_SUCCESS {
			fmt.Println("transaction is not success")
			continue
		}

		// if transaction is not tron transfer or erc20 transfer
		if transaction.RawData.Contract[0].Type != core.Transaction_Contract_TransferContract && transaction.RawData.Contract[0].Type != core.Transaction_Contract_TriggerSmartContract {
			continue
		}

		var crawlTransaction *CrawlTransaction = nil

		if transaction.RawData.Contract[0].Type == core.Transaction_Contract_TransferContract {
			contract := &core.TransferContract{}
			err := proto.Unmarshal(transaction.RawData.Contract[0].Parameter.Value, contract)
			if err != nil {
				fmt.Println(err)
				continue
			}
			crawlTransaction = c.prepareTrxTransaction(t, contract)
		} else if transaction.RawData.Contract[0].Type == core.Transaction_Contract_TriggerSmartContract {
			contract := &core.TriggerSmartContract{}
			err := proto.Unmarshal(transaction.RawData.Contract[0].Parameter.Value, contract)
			if err != nil {
				fmt.Println(err)
				continue
			}
			crawlTransaction = c.prepareTrc20Transaction(t, contract)
		}
		if crawlTransaction != nil {
			for _, ourAddress := range c.Addresses {
				if ourAddress == crawlTransaction.ContractAddress {
					txs = append(txs, *crawlTransaction)
				}
			}
		}

	}

	return txs
}

func (c *Crawler) prepareTrxTransaction(t *api.TransactionExtention, contract *core.TransferContract) *CrawlTransaction {

	// if address is hex convert to base58
	toAddress := hexutil.Encode(contract.ToAddress)[2:]
	if strings.HasPrefix(toAddress, "41") == true {
		toAddress = util.HexToAddress(toAddress).String()
	}

	// if address is hex convert to base58
	fromAddress := hexutil.Encode(contract.OwnerAddress)[2:]
	if strings.HasPrefix(fromAddress, "41") == true {
		fromAddress = util.HexToAddress(fromAddress).String()
	}

	return &CrawlTransaction{
		TxId:        hexutil.Encode(t.GetTxid())[2:],
		FromAddress: fromAddress,
		ToAddress:   toAddress,
		Amount:      contract.Amount,
		Symbol:      "TRX",
	}
}

func (c *Crawler) prepareTrc20Transaction(t *api.TransactionExtention, contract *core.TriggerSmartContract) *CrawlTransaction {

	tokenTransferData, validTokenData := util.ParseTrc20TokenTransfer(util.ToHex(contract.Data)[2:])

	if validTokenData == false {
		return nil
	}

	// if contractAddress is hex convert to base58
	contractAddress := hexutil.Encode(contract.ContractAddress)[2:]
	if strings.HasPrefix(contractAddress, "41") == true {
		contractAddress = util.HexToAddress(contractAddress).String()
	}

	// if address is hex convert to base58
	toAddress := tokenTransferData.To
	if strings.HasPrefix(toAddress, "41") == true {
		toAddress = util.HexToAddress(toAddress).String()
	}

	// if address is hex convert to base58
	fromAddress := hexutil.Encode(contract.OwnerAddress)[2:]
	if strings.HasPrefix(fromAddress, "41") == true {
		fromAddress = util.HexToAddress(fromAddress).String()
	}

	token := &Token{
		ContractAddress: enums.CreateContractAddress(contractAddress),
	}
	symbol, _ := token.GetSymbol(c.Node, fromAddress)

	return &CrawlTransaction{
		TxId:            hexutil.Encode(t.GetTxid())[2:],
		FromAddress:     fromAddress,
		ToAddress:       toAddress,
		Amount:          tokenTransferData.Value.Int64(),
		Symbol:          symbol,
		ContractAddress: contractAddress,
	}
}

func (c *Crawler) prepareCrawlResultFromTransactions(transactions [][]CrawlTransaction) []CrawlResult {

	var result []CrawlResult

	for _, transaction := range transactions {
		for _, tx := range transaction {

			if c.addressExistInResult(result, tx.ToAddress) {
				id, res := c.getAddressCrawlInResultList(result, tx.ToAddress)
				res.Transactions = append(res.Transactions, tx)
				result[id] = res

			} else {
				result = append(result, CrawlResult{
					Address:      tx.ToAddress,
					Transactions: []CrawlTransaction{tx},
				})
			}
		}
	}

	return result
}

func (c *Crawler) addressExistInResult(result []CrawlResult, address string) bool {
	for _, res := range result {
		if res.Address == address {
			return true
		}
	}
	return false
}

func (c *Crawler) getAddressCrawlInResultList(result []CrawlResult, address string) (int, CrawlResult) {
	for id, res := range result {
		if res.Address == address {
			return id, res
		}
	}
	panic("crawl result not found")
}
