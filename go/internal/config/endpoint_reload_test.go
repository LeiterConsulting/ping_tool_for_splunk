package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEndpointReloaderReloadsChangedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	writeEndpointsFixture(t, path,
		"ip,hostname,group,description,entitytype,device,vendor,additional_notes\n"+
			"127.0.0.1,host1,default,one,server,loopback,Microsoft,initial\n",
		time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC),
	)

	reloader, endpoints, err := NewEndpointReloader(path)
	if err != nil {
		t.Fatalf("NewEndpointReloader() error = %v", err)
	}
	if len(endpoints) != 1 || endpoints[0].Hostname != "host1" {
		t.Fatalf("unexpected initial endpoints: %#v", endpoints)
	}

	writeEndpointsFixture(t, path,
		"ip,hostname,group,description,entitytype,device,vendor,additional_notes\n"+
			"127.0.0.1,host1,default,one,server,loopback,Microsoft,initial\n"+
			"127.0.0.2,host2,default,two,server,loopback,Microsoft,updated\n",
		time.Date(2026, 5, 15, 10, 0, 2, 0, time.UTC),
	)

	reloaded, changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatalf("ReloadIfChanged() error = %v", err)
	}
	if !changed {
		t.Fatal("ReloadIfChanged() changed = false, want true")
	}
	if len(reloaded) != 2 || reloaded[1].Hostname != "host2" {
		t.Fatalf("unexpected reloaded endpoints: %#v", reloaded)
	}

	again, changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatalf("ReloadIfChanged() second call error = %v", err)
	}
	if changed {
		t.Fatal("ReloadIfChanged() second call changed = true, want false")
	}
	if len(again) != 2 {
		t.Fatalf("unexpected endpoints after no-op reload: %#v", again)
	}
}

func TestEndpointReloaderKeepsLastGoodSetOnInvalidFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	writeEndpointsFixture(t, path,
		"ip,hostname,group,description,entitytype,device,vendor,additional_notes\n"+
			"127.0.0.1,host1,default,one,server,loopback,Microsoft,initial\n",
		time.Date(2026, 5, 15, 11, 0, 0, 0, time.UTC),
	)

	reloader, endpoints, err := NewEndpointReloader(path)
	if err != nil {
		t.Fatalf("NewEndpointReloader() error = %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("unexpected initial endpoints: %#v", endpoints)
	}

	writeEndpointsFixture(t, path,
		"ip,hostname,group,description,entitytype,device,vendor,additional_notes\n",
		time.Date(2026, 5, 15, 11, 0, 2, 0, time.UTC),
	)

	reloaded, changed, err := reloader.ReloadIfChanged()
	if err == nil {
		t.Fatal("ReloadIfChanged() error = nil, want invalid CSV error")
	}
	if changed {
		t.Fatal("ReloadIfChanged() changed = true, want false on invalid file")
	}
	if len(reloaded) != 1 || reloaded[0].Hostname != "host1" {
		t.Fatalf("expected last good endpoints after invalid reload, got %#v", reloaded)
	}

	unchanged, changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatalf("ReloadIfChanged() repeated invalid file error = %v, want nil", err)
	}
	if changed {
		t.Fatal("ReloadIfChanged() repeated invalid file changed = true, want false")
	}
	if len(unchanged) != 1 || unchanged[0].Hostname != "host1" {
		t.Fatalf("expected last good endpoints after repeated invalid reload, got %#v", unchanged)
	}

	writeEndpointsFixture(t, path,
		"ip,hostname,group,description,entitytype,device,vendor,additional_notes\n"+
			"127.0.0.2,host2,default,two,server,loopback,Microsoft,recovered\n",
		time.Date(2026, 5, 15, 11, 0, 4, 0, time.UTC),
	)

	recovered, changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatalf("ReloadIfChanged() recovery error = %v", err)
	}
	if !changed {
		t.Fatal("ReloadIfChanged() recovery changed = false, want true")
	}
	if len(recovered) != 1 || recovered[0].Hostname != "host2" {
		t.Fatalf("unexpected endpoints after recovery: %#v", recovered)
	}
}

func writeEndpointsFixture(t *testing.T, path string, content string, modTime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("os.Chtimes(%s) error = %v", path, err)
	}
}