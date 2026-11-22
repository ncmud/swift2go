package types

import (
	"os/exec"
	"testing"
)

func TestNominalKinds(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs swift-demangle")
	}
	d, err := NewDemangler([]string{
		"s:4GRDB8DatabaseC10ColumnTypeV4textAEvpZ",
		"s:4GRDB11TableRecordPAAE6filteryAA21QueryInterfaceRequestVyxGqd__AA22SQLSpecificExpressibleRd__lFZ",
		"$s4GRDB8DatabaseC15TransactionKindOD",
		"$sSaySiGD",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.NominalKinds("GRDB")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]byte{
		"GRDB.Database":                 KindClass,
		"GRDB.Database.ColumnType":      KindStruct,
		"GRDB.Database.TransactionKind": KindEnum,
		"GRDB.TableRecord":              KindProtocol,
		"GRDB.QueryInterfaceRequest":    KindStruct,
		"GRDB.SQLSpecificExpressible":   KindProtocol,
	}
	for name, k := range want {
		if got[name] != k {
			t.Errorf("%s: kind %q, want %q", name, got[name], k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("kinds = %v", got)
	}
}
