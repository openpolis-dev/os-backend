package sdk

import (
	"reflect"
	"testing"
)

func TestParseIndexerSnsRegistryPayload_Array(t *testing.T) {
	body := []byte(`[{"owner":"0xAbCd","sns":"alice.seedao"},{"wallet":"0xef01","sns_name":"bob"}]`)
	got, err := parseIndexerSnsRegistryPayload(body)
	if err != nil {
		t.Fatalf("parseIndexerSnsRegistryPayload() error = %v", err)
	}
	want := []SnsRegistryRecord{
		{Wallet: "0xabcd", SnsName: "alice.seedao"},
		{Wallet: "0xef01", SnsName: "bob"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestParseIndexerSnsRegistryPayload_EmptyObject(t *testing.T) {
	got, err := parseIndexerSnsRegistryPayload([]byte(`{}`))
	if err != nil {
		t.Fatalf("parseIndexerSnsRegistryPayload() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got=%v", got)
	}
}

func TestParseIndexerSnsRegistryPayload_WrappedData(t *testing.T) {
	body := []byte(`{"data":[{"owner":"0x1111","sns":"foo.seedao"}]}`)
	got, err := parseIndexerSnsRegistryPayload(body)
	if err != nil {
		t.Fatalf("parseIndexerSnsRegistryPayload() error = %v", err)
	}
	if len(got) != 1 || got[0].Wallet != "0x1111" || got[0].SnsName != "foo.seedao" {
		t.Fatalf("unexpected result: %+v", got)
	}
}
