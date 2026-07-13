package portforwarding

import "testing"

func TestOptionalComputedCollectionsNormalizeOmittedFieldsToEmpty(t *testing.T) {
	list := stringsToList(nil)
	if list.IsNull() || list.IsUnknown() || len(list.Elements()) != 0 {
		t.Fatalf("stringsToList(nil) = %v, want known empty list", list)
	}

	valueMap := portIpsToMap(nil)
	if valueMap.IsNull() || valueMap.IsUnknown() || len(valueMap.Elements()) != 0 {
		t.Fatalf("portIpsToMap(nil) = %v, want known empty map", valueMap)
	}
}
