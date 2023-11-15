// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package generated

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// SeedMetaData contains all meta data concerning the Seed contract.
var SeedMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// SeedABI is the input ABI used to generate the binding from.
// Deprecated: Use SeedMetaData.ABI instead.
var SeedABI = SeedMetaData.ABI

// Seed is an auto generated Go binding around an Ethereum contract.
type Seed struct {
	SeedCaller     // Read-only binding to the contract
	SeedTransactor // Write-only binding to the contract
	SeedFilterer   // Log filterer for contract events
}

// SeedCaller is an auto generated read-only Go binding around an Ethereum contract.
type SeedCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SeedTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SeedTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SeedFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SeedFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SeedSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SeedSession struct {
	Contract     *Seed             // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SeedCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SeedCallerSession struct {
	Contract *SeedCaller   // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// SeedTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SeedTransactorSession struct {
	Contract     *SeedTransactor   // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SeedRaw is an auto generated low-level Go binding around an Ethereum contract.
type SeedRaw struct {
	Contract *Seed // Generic contract binding to access the raw methods on
}

// SeedCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SeedCallerRaw struct {
	Contract *SeedCaller // Generic read-only contract binding to access the raw methods on
}

// SeedTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SeedTransactorRaw struct {
	Contract *SeedTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSeed creates a new instance of Seed, bound to a specific deployed contract.
func NewSeed(address common.Address, backend bind.ContractBackend) (*Seed, error) {
	contract, err := bindSeed(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Seed{SeedCaller: SeedCaller{contract: contract}, SeedTransactor: SeedTransactor{contract: contract}, SeedFilterer: SeedFilterer{contract: contract}}, nil
}

// NewSeedCaller creates a new read-only instance of Seed, bound to a specific deployed contract.
func NewSeedCaller(address common.Address, caller bind.ContractCaller) (*SeedCaller, error) {
	contract, err := bindSeed(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SeedCaller{contract: contract}, nil
}

// NewSeedTransactor creates a new write-only instance of Seed, bound to a specific deployed contract.
func NewSeedTransactor(address common.Address, transactor bind.ContractTransactor) (*SeedTransactor, error) {
	contract, err := bindSeed(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SeedTransactor{contract: contract}, nil
}

// NewSeedFilterer creates a new log filterer instance of Seed, bound to a specific deployed contract.
func NewSeedFilterer(address common.Address, filterer bind.ContractFilterer) (*SeedFilterer, error) {
	contract, err := bindSeed(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SeedFilterer{contract: contract}, nil
}

// bindSeed binds a generic wrapper to an already deployed contract.
func bindSeed(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(SeedABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Seed *SeedRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Seed.Contract.SeedCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Seed *SeedRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Seed.Contract.SeedTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Seed *SeedRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Seed.Contract.SeedTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Seed *SeedCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Seed.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Seed *SeedTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Seed.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Seed *SeedTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Seed.Contract.contract.Transact(opts, method, params...)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Seed *SeedCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Seed.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Seed *SeedSession) TotalSupply() (*big.Int, error) {
	return _Seed.Contract.TotalSupply(&_Seed.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Seed *SeedCallerSession) TotalSupply() (*big.Int, error) {
	return _Seed.Contract.TotalSupply(&_Seed.CallOpts)
}
