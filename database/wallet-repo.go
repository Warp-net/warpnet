// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const WalletRepoName = "/WALLETS"

var ErrWalletAddressNotFound = local_store.DBError("wallet address not found")

type WalletStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type WalletRepo struct {
	db WalletStorer
}

func NewWalletRepo(db WalletStorer) *WalletRepo {
	return &WalletRepo{db: db}
}

func walletAddressKey(chain, userId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(WalletRepoName).
		AddSubPrefix(chain).
		AddRootID(userId).
		Build()
}

func (repo *WalletRepo) SetAddress(chain, userId, address string) error {
	if chain == "" {
		return local_store.DBError("empty chain")
	}
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if address == "" {
		return local_store.DBError("empty wallet address")
	}

	value, err := json.Marshal(domain.WalletAddress{
		Address:   address,
		Chain:     chain,
		UpdatedAt: time.Now(),
		UserId:    userId,
	})
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Set(walletAddressKey(chain, userId), value); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *WalletRepo) GetAddress(chain, userId string) (domain.WalletAddress, error) {
	if chain == "" {
		return domain.WalletAddress{}, local_store.DBError("empty chain")
	}
	if userId == "" {
		return domain.WalletAddress{}, local_store.DBError("empty user id")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.WalletAddress{}, err
	}
	defer txn.Rollback()

	value, err := txn.Get(walletAddressKey(chain, userId))
	if local_store.IsNotFoundError(err) {
		return domain.WalletAddress{}, ErrWalletAddressNotFound
	}
	if err != nil {
		return domain.WalletAddress{}, err
	}

	var stored domain.WalletAddress
	if err := json.Unmarshal(value, &stored); err != nil {
		return domain.WalletAddress{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.WalletAddress{}, err
	}
	return stored, nil
}

func (repo *WalletRepo) ListAddresses(chain string, limit *uint64, cursor *string) ([]domain.WalletAddress, string, error) {
	if chain == "" {
		return nil, "", local_store.DBError("empty chain")
	}
	prefix := local_store.NewPrefixBuilder(WalletRepoName).
		AddSubPrefix(chain).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, next, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	addresses := make([]domain.WalletAddress, 0, len(items))
	for _, item := range items {
		var stored domain.WalletAddress
		if err := json.Unmarshal(item.Value, &stored); err != nil {
			continue
		}
		addresses = append(addresses, stored)
	}
	return addresses, next, nil
}
