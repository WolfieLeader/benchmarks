package container

import "testing"

func TestMemoryLimitBytes(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"2gb", 2 << 30, false},
		{"512mb", 512 << 20, false},
		{"1024kb", 1024 << 10, false},
		{"1024", 1024, false},
		{"1.5gb", int64(1.5 * (1 << 30)), false},
		{"0", 0, true},
		{"abc", 0, true},
		{"-1mb", 0, true},
	}
	for _, tt := range tests {
		got, err := memoryLimitBytes(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("memoryLimitBytes(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("memoryLimitBytes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
