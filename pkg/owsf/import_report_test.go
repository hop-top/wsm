package owsf

import "testing"

func TestImportReport_Downgrade(t *testing.T) {
	exp := Export{
		Kind:           ExportPartial,
		Determinism:    DeterminismFull,
		MaxConformance: 3,
		SliceScope:     &SliceScope{SpaceIDs: []string{"s"}},
	}
	report := NewImportReport(exp, 2)
	if !report.Downgraded {
		t.Fatal("should detect downgrade")
	}
	if report.ActualConformance != 2 {
		t.Fatal("actual conformance mismatch")
	}
}

func TestImportReport_NoDowngrade(t *testing.T) {
	exp := Export{
		Kind:           ExportFull,
		Determinism:    DeterminismFull,
		MaxConformance: 1,
	}
	report := NewImportReport(exp, 1)
	if report.Downgraded {
		t.Fatal("no downgrade expected")
	}
}
