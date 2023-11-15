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

// SCRMetaData contains all meta data concerning the SCR contract.
var SCRMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// SCRABI is the input ABI used to generate the binding from.
// Deprecated: Use SCRMetaData.ABI instead.
var SCRABI = SCRMetaData.ABI

// SCR is an auto generated Go binding around an Ethereum contract.
type SCR struct {
	SCRCaller     // Read-only binding to the contract
	SCRTransactor // Write-only binding to the contract
	SCRFilterer   // Log filterer for contract events
}

// SCRCaller is an auto generated read-only Go binding around an Ethereum contract.
type SCRCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SCRTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SCRTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SCRFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SCRFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SCRSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SCRSession struct {
	Contract     *SCR              // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SCRCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SCRCallerSession struct {
	Contract *SCRCaller    // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// SCRTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SCRTransactorSession struct {
	Contract     *SCRTransactor    // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SCRRaw is an auto generated low-level Go binding around an Ethereum contract.
type SCRRaw struct {
	Contract *SCR // Generic contract binding to access the raw methods on
}

// SCRCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SCRCallerRaw struct {
	Contract *SCRCaller // Generic read-only contract binding to access the raw methods on
}

// SCRTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SCRTransactorRaw struct {
	Contract *SCRTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSCR creates a new instance of SCR, bound to a specific deployed contract.
func NewSCR(address common.Address, backend bind.ContractBackend) (*SCR, error) {
	contract, err := bindSCR(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SCR{SCRCaller: SCRCaller{contract: contract}, SCRTransactor: SCRTransactor{contract: contract}, SCRFilterer: SCRFilterer{contract: contract}}, nil
}

// NewSCRCaller creates a new read-only instance of SCR, bound to a specific deployed contract.
func NewSCRCaller(address common.Address, caller bind.ContractCaller) (*SCRCaller, error) {
	contract, err := bindSCR(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SCRCaller{contract: contract}, nil
}

// NewSCRTransactor creates a new write-only instance of SCR, bound to a specific deployed contract.
func NewSCRTransactor(address common.Address, transactor bind.ContractTransactor) (*SCRTransactor, error) {
	contract, err := bindSCR(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SCRTransactor{contract: contract}, nil
}

// NewSCRFilterer creates a new log filterer instance of SCR, bound to a specific deployed contract.
func NewSCRFilterer(address common.Address, filterer bind.ContractFilterer) (*SCRFilterer, error) {
	contract, err := bindSCR(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SCRFilterer{contract: contract}, nil
}

// bindSCR binds a generic wrapper to an already deployed contract.
func bindSCR(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(SCRABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SCR *SCRRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SCR.Contract.SCRCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SCR *SCRRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SCR.Contract.SCRTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SCR *SCRRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SCR.Contract.SCRTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SCR *SCRCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SCR.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SCR *SCRTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SCR.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SCR *SCRTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SCR.Contract.contract.Transact(opts, method, params...)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_SCR *SCRCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SCR.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_SCR *SCRSession) TotalSupply() (*big.Int, error) {
	return _SCR.Contract.TotalSupply(&_SCR.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_SCR *SCRCallerSession) TotalSupply() (*big.Int, error) {
	return _SCR.Contract.TotalSupply(&_SCR.CallOpts)
}
