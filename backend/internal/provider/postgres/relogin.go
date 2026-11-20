package postgres

import (
	"context"
	"slices"

	entsql "entgo.io/ent/dialect/sql"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	dbprovider "github.com/TokenFlux/TokenRouter/ent/provider"
	dbprovidergroup "github.com/TokenFlux/TokenRouter/ent/providergroup"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

// FindReloginProviders 在未删除的 OpenAI OAuth 提供商中按完整邮箱查找。
func (r *ProviderStore) FindReloginProviders(ctx context.Context, email string) ([]providercore.Record, error) {
	matches, err := r.client.Provider.Query().
		Where(
			dbprovider.DeletedAtIsNil(),
			dbprovider.PlatformEQ(providercore.PlatformOpenAI),
			dbprovider.TypeEQ(providercore.ProviderTypeOAuth),
			dbprovider.Or(
				dbprovider.NameContainsFold(email),
				func(s *entsql.Selector) {
					s.Where(entsql.P(func(b *entsql.Builder) {
						b.WriteString("lower(btrim(").Ident(s.C(dbprovider.FieldCredentials)).WriteString(" ->> 'email')) = ").Arg(email)
						b.WriteString(" OR lower(btrim(").Ident(s.C(dbprovider.FieldExtra)).WriteString(" ->> 'email')) = ").Arg(email)
					}))
				},
			),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	confirmed := matches[:0]
	for _, candidate := range matches {
		if providercore.MatchesReloginEmail(r.recordFromEntity(candidate), email) {
			confirmed = append(confirmed, candidate)
		}
	}
	return r.RecordsFromEntities(ctx, confirmed)
}

// RestoreReloginSettings 在同一事务核对账号、恢复配置与分组，并写入调度事件。
func (r *ProviderStore) RestoreReloginSettings(
	ctx context.Context,
	id int64,
	email, credentialVersion string,
	expected, desired providercore.ReloginSettings,
) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()
	entity, err := client.Provider.Query().
		Where(dbprovider.IDEQ(id), dbprovider.DeletedAtIsNil()).
		ForUpdate().
		Only(ctx)
	if dbent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	current := r.recordFromEntity(entity)
	if current.Status != providercore.StatusActive || !providercore.MatchesReloginEmail(current, email) ||
		credentialVersion == "" || providercore.RefreshCredentialIdentity(current) != credentialVersion {
		return false, nil
	}
	groups, err := client.ProviderGroup.Query().
		Where(dbprovidergroup.ProviderIDEQ(id)).
		Order(dbent.Asc(dbprovidergroup.FieldGroupID)).
		ForUpdate().
		All(ctx)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		current.GroupIDs = append(current.GroupIDs, group.GroupID)
	}
	if !expected.Matches(current) {
		return false, nil
	}

	// 到期判断使用写入事务开始调度配置时的时钟。
	schedulable := desired.Schedulable && (desired.ExpiresAt == nil || r.options.Now().Before(*desired.ExpiresAt))
	update := client.Provider.UpdateOneID(id).
		SetPriority(desired.Priority).
		SetConcurrency(desired.Concurrency).
		SetAutoPauseOnExpired(desired.AutoPauseOnExpired).
		SetSchedulable(schedulable)
	if desired.LoadFactor == nil {
		update.ClearLoadFactor()
	} else {
		update.SetLoadFactor(*desired.LoadFactor)
	}
	if desired.ExpiresAt == nil {
		update.ClearExpiresAt()
	} else {
		update.SetExpiresAt(*desired.ExpiresAt)
	}
	if _, err := update.Save(ctx); err != nil {
		return false, err
	}

	if !slices.Equal(current.GroupIDs, desired.GroupIDs) {
		if err := replaceReloginGroups(ctx, client, id, current.GroupIDs, desired.GroupIDs); err != nil {
			return false, err
		}
	}
	if err := r.publish(ctx, client, id, MergeGroupIDs(current.GroupIDs, desired.GroupIDs)); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.afterChangeDetached(ctx, id)
	return true, nil
}

func replaceReloginGroups(ctx context.Context, client *dbent.Client, id int64, current, desired []int64) error {
	toRemove := make([]int64, 0, len(current))
	for _, groupID := range current {
		if !slices.Contains(desired, groupID) {
			toRemove = append(toRemove, groupID)
		}
	}
	if len(toRemove) > 0 {
		if _, err := client.ProviderGroup.Delete().
			Where(dbprovidergroup.ProviderIDEQ(id), dbprovidergroup.GroupIDIn(toRemove...)).
			Exec(ctx); err != nil {
			return err
		}
	}
	toAdd := make([]*dbent.ProviderGroupCreate, 0, len(desired))
	for _, groupID := range desired {
		if !slices.Contains(current, groupID) {
			toAdd = append(toAdd, client.ProviderGroup.Create().SetProviderID(id).SetGroupID(groupID))
		}
	}
	if len(toAdd) > 0 {
		_, err := client.ProviderGroup.CreateBulk(toAdd...).Save(ctx)
		return err
	}
	return nil
}
