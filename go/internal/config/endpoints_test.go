package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEndpoints_ParsesDevColumn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	content := "ip,hostname,group,description,entitytype,device,vendor,additional_notes,dev\n" +
		"10.0.0.1,dev-api,dev,Development API,service,api,Acme,internal,true\n" +
		"10.0.0.2,prod-api,prod,Production API,service,api,Acme,customer-facing,false\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	eps, err := LoadEndpoints(path)
	if err != nil {
		t.Fatalf("LoadEndpoints() error = %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("LoadEndpoints() count = %d, want 2", len(eps))
	}
	if !eps[0].Dev {
		t.Fatal("expected first endpoint dev=true")
	}
	if eps[1].Dev {
		t.Fatal("expected second endpoint dev=false")
	}
}

func TestLoadEndpoints_MissingDevColumnDefaultsFalse(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	content := "ip,hostname,group\n" +
		"10.0.0.1,host-a,default\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	eps, err := LoadEndpoints(path)
	if err != nil {
		t.Fatalf("LoadEndpoints() error = %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("LoadEndpoints() count = %d, want 1", len(eps))
	}
	if eps[0].Dev {
		t.Fatal("expected missing dev column to default false")
	}
}

func TestLoadEndpoints_RejectsIncompleteRecord(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "endpoints.csv")
	content := "ip,hostname,group\n10.0.0.1,host-a,default\n10.0.0.2,,default\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEndpoints(path); err == nil || !strings.Contains(err.Error(), "record 3") {
		t.Fatalf("LoadEndpoints() error = %v, want record 3 failure", err)
	}
}

func TestLoadEndpoints_RejectsDuplicateCanonicalIP(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "endpoints.csv")
	content := "ip,hostname\n2001:db8::1,host-a\n2001:0db8:0:0:0:0:0:1,host-b\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEndpoints(path); err == nil || !strings.Contains(err.Error(), "duplicates target IP") {
		t.Fatalf("LoadEndpoints() error = %v, want duplicate IP failure", err)
	}
}

func TestLoadEndpoints_RejectsInvalidIPAndDevFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "dns name", content: "ip,hostname\nrouter.example.test,router\n", want: "invalid IP address"},
		{name: "dev flag", content: "ip,hostname,dev\n10.0.0.1,router,maybe\n", want: "invalid dev value"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "endpoints.csv")
			if err := os.WriteFile(path, []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadEndpoints(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadEndpoints() error = %v, want %q", err, test.want)
			}
		})
	}
}
