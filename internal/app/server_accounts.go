package app

import (
	"context"
	"errors"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/api"
)

// An exclusive native lease waits for old requests to finish before replacing
// credentials. It does not verify a Google session or initialize a worker.
func (admin *runtimeAdmin) replaceImportedState(ctx context.Context, id string, state aistudio.StorageState) (api.AdminAccount, error) {
	lease, err := admin.pool.AcquireAccount(ctx, id)
	if err != nil {
		return api.AdminAccount{}, accountOperationError(err)
	}
	if err := admin.workers.Reset(id); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := lease.SaveStorageState(state); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := lease.Release(); err != nil {
		return api.AdminAccount{}, err
	}
	admin.requests.log("auth", "INFO", "账户凭据已更新（仅保存）")
	return admin.account(id)
}
