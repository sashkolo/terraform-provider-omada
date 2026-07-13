package portforwarding

import "testing"

func TestListComplete(t *testing.T) {
	tests := []struct {
		name        string
		pageRows    int
		accumulated int
		totalRows   *int64
		want        bool
	}{
		{
			name:        "missing total and full page continues",
			pageRows:    int(listPageSize),
			accumulated: int(listPageSize),
			want:        false,
		},
		{
			name:        "missing total and short page completes",
			pageRows:    12,
			accumulated: 12,
			want:        true,
		},
		{
			name:        "known total reached completes",
			pageRows:    int(listPageSize),
			accumulated: int(listPageSize),
			totalRows:   int64Pointer(int64(listPageSize)),
			want:        true,
		},
		{
			name:        "known total not reached continues on full page",
			pageRows:    int(listPageSize),
			accumulated: int(listPageSize),
			totalRows:   int64Pointer(int64(listPageSize) + 1),
			want:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := listComplete(test.pageRows, test.accumulated, test.totalRows); got != test.want {
				t.Fatalf("listComplete(%d, %d, %v) = %t, want %t", test.pageRows, test.accumulated, test.totalRows, got, test.want)
			}
		})
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
