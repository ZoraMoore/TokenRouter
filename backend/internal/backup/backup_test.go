package backup

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	TestSettingKeyBackupS3Config      = settingKeyBackupS3Config
	TestSettingKeyBackupStorageConfig = settingKeyBackupStorageConfig
	TestSettingKeyBackupRecords       = settingKeyBackupRecords
)

func TestRestoreMustRegister(t *testing.T) {
	repo := &runtimeSettings{}
	archive := &runtimeArchive{}
	s := runtimeBackup(repo, archive)
	data, err := json.Marshal([]BackupRecord{{ID: "fixture", Status: "completed", StorageType: "local", StorageKey: "fixture.gz"}})
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), settingKeyBackupRecords, string(data)))
	repo.fail = true
	_, err = s.StartRestore(context.Background(), "fixture")
	require.Error(t, err)
	s.Stop()
	require.Zero(t, archive.calls.Load())
}

func AccessSaveRecord(s *BackupService, c context.Context, r *BackupRecord) error {
	return s.saveRecord(c, r)
}

func AccessLoadRecords(s *BackupService, c context.Context) ([]BackupRecord, error) {
	return s.loadRecords(c)
}

func AccessLoadS3(s *BackupService, c context.Context) (*BackupS3Config, error) {
	return s.loadS3Config(c)
}

func AccessRecover(s *BackupService) { s.recoverStaleRecords() }

func AccessWait(s *BackupService) { s.wg.Wait() }

func AccessBackingUp(s *BackupService) { s.opMu.Lock(); s.backingUp = true; s.opMu.Unlock() }

func AccessArchive(s *BackupService) ArchiveExecutor { return s.archive }
