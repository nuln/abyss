package abyss

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testShardedProvider struct{}

func (testShardedProvider) Info() PluginInfo {
	return PluginInfo{SlugName: "sharded-test-provider", Name: "sharded-test-provider"}
}

func (testShardedProvider) AvailableStorageTypes() []StorageTypeInfo {
	return []StorageTypeInfo{{Name: "sharded", DisplayName: "sharded", Description: "test sharded"}}
}

func (testShardedProvider) CreateUserEngine(uint64) (StorageEngine, error) {
	return testShardedEngine{}, nil
}

func (testShardedProvider) OnUserEngineInit(uint64, func(string, StorageEngine)) error {
	return nil
}

func (testShardedProvider) ResolveVirtualPath(context.Context, uint64, string) (*VirtualPathInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual path.
}

func (testShardedProvider) GetVirtualEntries(context.Context, uint64) ([]*EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual entries.
}

func (testShardedProvider) MigrateStorage(context.Context, string, string, func(uint64) (StorageEngine, error)) error {
	return nil
}

func (testShardedProvider) PreflightMigration(context.Context, string, string) error {
	return nil
}

func (testShardedProvider) TUSUploadComplete(context.Context, uint64, string, ReadSeekCloser) error {
	return nil
}

type testShardedEngine struct{}

func (testShardedEngine) Name() string { return "sharded" }

func (testShardedEngine) Read(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Stat(context.Context, string) (*FileStat, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Write(context.Context, string, io.Reader) error { return nil }
func (testShardedEngine) Mkdir(context.Context, string) error            { return nil }
func (testShardedEngine) Copy(context.Context, string, string) error     { return nil }
func (testShardedEngine) Move(context.Context, string, string) error     { return nil }
func (testShardedEngine) Delete(context.Context, string) error           { return nil }

func (testShardedEngine) List(context.Context, string) ([]EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no entries.
}

func TestStorageSwitchToSharded(t *testing.T) {
	RegisterStorageProvider(testShardedProvider{})
	_ = StatusManager.Enable("sharded-test-provider", true)

	db := openTestDB(t)

	userStore := &boltUserStore{db: db}
	settingsStore := &boltSettingsStore{db: db}
	fileStore := &boltFileStore{db: db}
	settingsSvc := newSettingsService(settingsStore)

	dataDir, err := os.MkdirTemp("", "abyss-storage-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dataDir)

	storageSvc := newStorageService(fileStore, userStore, settingsSvc, dataDir)

	user := &User{Email: "test@example.com", Username: "sharded_test_user", UUID: "sharded-test-uuid"}
	err = userStore.Create(context.Background(), user)
	require.NoError(t, err)
	require.NotZero(t, user.ID)

	engine, err := storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "path", engine.Name())

	settings, err := settingsSvc.Get(context.Background())
	require.NoError(t, err)
	settings.StorageType = "sharded"
	err = settingsSvc.Save(context.Background(), settings)
	require.NoError(t, err)

	storageSvc.engines = make(map[uint64]StorageEngine)

	engine, err = storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "sharded", engine.Name())
}
