package metaforo

import (
	"testing"
)

func TestToken(t *testing.T) {
	//got, err := Token("5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a", "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC", "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	got, err := Token("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", "0x70997970C51812dc3A010C7d01b50e0d17dc79C8", "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", got)
}
