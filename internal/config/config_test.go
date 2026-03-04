package config

import (
	"path/filepath"
	"testing"

	"gids/internal/testutil"
)

func TestLoad_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestLoad_CreatesNoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "config.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil AppConfig")
	}
}

func TestSave_And_Load_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	want := &AppConfig{
		Profiles: []Profile{
			{
				Name:      testutil.ProfileName,
				GitName:   testutil.GitName,
				GitEmail:  testutil.GitEmail,
				Username:  testutil.Username,
				SSHKey:    testutil.SSHKey,
				SigningKey: testutil.SigningKey,
			},
		},
	}

	if err := Save(want, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(got.Profiles))
	}
	p := got.Profiles[0]
	if p.Name != testutil.ProfileName ||
		p.GitName != testutil.GitName ||
		p.GitEmail != testutil.GitEmail ||
		p.Username != testutil.Username ||
		p.SSHKey != testutil.SSHKey ||
		p.SigningKey != testutil.SigningKey {
		t.Errorf("roundtrip mismatch: %+v", p)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.yaml")

	cfg := &AppConfig{}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestSave_MultipleProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &AppConfig{
		Profiles: []Profile{
			{Name: testutil.ProfileName, GitName: testutil.GitName, GitEmail: testutil.GitEmail},
			{Name: testutil.ProfileName2, GitName: testutil.GitName, GitEmail: testutil.GitEmail2},
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(got.Profiles))
	}
}

func TestFindProfile(t *testing.T) {
	cfg := &AppConfig{
		Profiles: []Profile{
			{Name: testutil.ProfileName, GitName: testutil.GitName},
			{Name: testutil.ProfileName2, GitName: testutil.GitName},
		},
	}

	p, idx := cfg.FindProfile(testutil.ProfileName)
	if p == nil || idx != 0 {
		t.Errorf("expected %q at index 0, got idx=%d", testutil.ProfileName, idx)
	}

	p, idx = cfg.FindProfile("Missing")
	if p != nil || idx != -1 {
		t.Errorf("expected nil/-1 for missing profile, got idx=%d", idx)
	}
}

func TestDeleteProfile(t *testing.T) {
	cfg := &AppConfig{
		Profiles: []Profile{
			{Name: testutil.ProfileName},
			{Name: testutil.ProfileName2},
		},
	}

	ok := cfg.DeleteProfile(testutil.ProfileName)
	if !ok {
		t.Fatal("expected true when deleting existing profile")
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != testutil.ProfileName2 {
		t.Errorf("unexpected profiles after delete: %v", cfg.Profiles)
	}

	ok = cfg.DeleteProfile("Missing")
	if ok {
		t.Fatal("expected false when deleting non-existent profile")
	}
}
