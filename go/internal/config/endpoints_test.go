package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEndpoints_ParsesDevColumn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	content := "ip,hostname,dev,group,description,entitytype,device,vendor,additional_notes\n" +
		"10.0.0.1,dev-api,true,dev,Development API,service,api,Acme,internal\n" +
		"10.0.0.2,prod-api,false,prod,Production API,service,api,Acme,customer-facing\n"
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
