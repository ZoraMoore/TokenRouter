package postgres

import (
	"context"

	dbent "github.com/TokenFlux/TokenRouter/ent"
)

// GroupAccessParticipant 在调用方的事务中操作授权记录。
type GroupAccessParticipant struct {
	tx    *dbent.Tx
	store *UserStore
}

func GroupAccessInTx(tx *dbent.Tx) *GroupAccessParticipant {
	return &GroupAccessParticipant{tx: tx, store: &UserStore{client: tx.Client()}}
}

func (p *GroupAccessParticipant) AddGroupToAllowedGroups(ctx context.Context, userID, groupID int64) error {
	return p.store.AddGroupToAllowedGroups(dbent.NewTxContext(ctx, p.tx), userID, groupID)
}
