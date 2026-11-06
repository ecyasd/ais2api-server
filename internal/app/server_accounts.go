package app

import (
	"context"
	"errors"

	"github.com/Mag1cFall/AIStudio2API/internal/api"
)

// An exclusive native lease waits for old requests to finish before replacing
// credentials. It does not verify a Google session or initialize a worker.
func (admin *runtimeAdmin) replaceImportedState(ctx context.Context, id string, input api.AccountStateInput) (api.AdminAccount, error) {
	lease, err := admin.pool.AcquireAccount(ctx, id)
	if err != nil {
		return api.AdminAccount{}, accountOperationError(err)
	}
	state := input.StorageState
	account := lease.Account()
	if err := keepAuthExtension(&state, account.StorageState); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	cfg := account.Config
	if input.AuthUserProvided || input.RenewalToken != "" {
		cfg.AuthUser = input.AuthUser
	}
	if input.Locale != "" {
		cfg.Locale = input.Locale
	}
	if input.Timezone != "" {
		cfg.Timezone = input.Timezone
	}
	update, err := admin.workers.prepareUpdate(account, cfg)
	if err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	committed := false
	defer func() {
		if !committed {
			update.Discard()
		}
	}()
	if err := lease.SaveStorageState(state); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := lease.SaveConfig(cfg); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := updateServerAccountMetadata(account.Directory, func(meta *serverAccountMetadata) { meta.ImportCompleteness = input.ImportCompleteness }); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if input.RenewalToken != "" {
		if err := admin.savePairingRenewal(id, input.RenewalToken); err != nil {
			return api.AdminAccount{}, errors.Join(err, lease.Release())
		}
	}
	if err := admin.service.changeModels(func() error { return admin.pool.MarkReady(id) }); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	update.Commit()
	committed = true
	if err := lease.Release(); err != nil {
		return api.AdminAccount{}, err
	}
	admin.requests.log("auth", "INFO", "账户凭据已更新（仅保存）")
	return admin.account(id)
}
