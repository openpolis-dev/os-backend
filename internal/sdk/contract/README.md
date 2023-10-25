
install `solc` and `abigen` first:

```
$ npm install -g solc
$ go install github.com/ethereum/go-ethereum/cmd/abigen@latest
```

and then execute code generation:

```
$ abigen --abi abi/SCR.json --pkg generated --type SCR --out generated/scr.go
$ abigen --abi abi/Seed.json --pkg generated --type Seed --out generated/seed.go
$ abigen --abi abi/Node.json --pkg generated --type Node --out generated/node.go
```
