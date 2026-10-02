package network

import (
	"reflect"
	"testing"
)

func TestInterfaceNamesUsesMACNotEnumeration(t *testing.T) {
	names, err := interfaceNames([]byte(`[{"ifname":"eth10","address":"02:00:00:00:00:03"},{"ifname":"eth8","address":"02:00:00:00:00:01"},{"ifname":"lo","address":"00:00:00:00:00:00"},{"ifname":"eth9","address":"02:00:00:00:00:02"}]`), []string{"02:00:00:00:00:01", "02:00:00:00:00:02", "02:00:00:00:00:03"})
	if err != nil || !reflect.DeepEqual(names, []string{"eth8", "eth9", "eth10"}) {
		t.Fatalf("%v %v", names, err)
	}
}

func TestInterfaceNamesRejectsMissingAmbiguousAndMalformed(t *testing.T) {
	for _, data := range []string{`[]`, `null`, `{}`, `[`, `[{"ifname":"","address":"02:00:00:00:00:01"}]`, `[{"ifname":"eth8","address":"02:00:00:00:00:01"},{"ifname":"br0","address":"02:00:00:00:00:01"}]`} {
		if _, err := interfaceNames([]byte(data), []string{"02:00:00:00:00:01"}); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
