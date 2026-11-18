package postgres

import (
	"context"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/tlsfingerprintprofile"
	"github.com/TokenFlux/TokenRouter/internal/egress"
)

type tlsFingerprintProfileRepository struct {
	client *ent.Client
}

// NewTLSFingerprintProfileRepository 创建 TLS 指纹模板仓库。
func NewTLSFingerprintProfileRepository(client *ent.Client) egress.TLSFingerprintProfileRepository {
	return &tlsFingerprintProfileRepository{client: client}
}

// List 查询可供选择和缓存的模板。
func (r *tlsFingerprintProfileRepository) List(ctx context.Context) ([]*egress.TLSFingerprintProfile, error) {
	profiles, err := r.client.TLSFingerprintProfile.Query().
		Where(allowStoredTLSProfile).
		Order(ent.Asc(tlsfingerprintprofile.FieldName)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*egress.TLSFingerprintProfile, len(profiles))
	for i, p := range profiles {
		result[i] = r.toModel(p)
	}
	return result, nil
}

// allowStoredTLSProfile 将早期采集功能留下的未发布模板排除在选择列表和运行缓存外。
// 历史列由迁移 259 创建，配置数据继续保存在原模板行。
func allowStoredTLSProfile(selector *sql.Selector) {
	const metadata = "verification"
	source := sqljson.Path("source_kind")
	published := sqljson.ValueEQ(metadata, true, sqljson.Path("approved_for_use"))
	if selector.Dialect() == dialect.Postgres {
		published = sqljson.ValueContains(metadata, map[string]bool{"approved_for_use": true})
	}
	selector.Where(sql.Or(
		sql.Not(sqljson.HasKey(metadata, source)),
		sqljson.ValueIsNull(metadata, source),
		sql.And(
			sqljson.ValueNEQ(metadata, "authenticated_ingress", source),
			sqljson.ValueNEQ(metadata, "controlled_connect", source),
		),
		sql.And(
			sqljson.ValueEQ(metadata, "verified", sqljson.Path("validation_status")),
			published,
		),
	))
}

// GetByID 根据 ID 获取模板。
func (r *tlsFingerprintProfileRepository) GetByID(ctx context.Context, id int64) (*egress.TLSFingerprintProfile, error) {
	p, err := r.client.TLSFingerprintProfile.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return r.toModel(p), nil
}

// Create 创建模板。
func (r *tlsFingerprintProfileRepository) Create(ctx context.Context, p *egress.TLSFingerprintProfile) (*egress.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.Create().
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
	}
	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
	}
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
	}
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
	}
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
	}
	if len(p.SignatureAlgorithmsCert) > 0 {
		builder.SetSignatureAlgorithmsCert(p.SignatureAlgorithmsCert)
	}
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
	}
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
	}
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
	}
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
	}
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(created), nil
}

// Update 更新模板。
func (r *tlsFingerprintProfileRepository) Update(ctx context.Context, p *egress.TLSFingerprintProfile) (*egress.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.UpdateOneID(p.ID).
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
	} else {
		builder.ClearDescription()
	}

	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
	} else {
		builder.ClearCipherSuites()
	}
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
	} else {
		builder.ClearCurves()
	}
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
	} else {
		builder.ClearPointFormats()
	}
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
	} else {
		builder.ClearSignatureAlgorithms()
	}
	if len(p.SignatureAlgorithmsCert) > 0 {
		builder.SetSignatureAlgorithmsCert(p.SignatureAlgorithmsCert)
	} else {
		builder.ClearSignatureAlgorithmsCert()
	}
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
	} else {
		builder.ClearAlpnProtocols()
	}
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
	} else {
		builder.ClearSupportedVersions()
	}
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
	} else {
		builder.ClearKeyShareGroups()
	}
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
	} else {
		builder.ClearPskModes()
	}
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
	} else {
		builder.ClearExtensions()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(updated), nil
}

// Delete 删除模板。
func (r *tlsFingerprintProfileRepository) Delete(ctx context.Context, id int64) error {
	return r.client.TLSFingerprintProfile.DeleteOneID(id).Exec(ctx)
}

// toModel 将 Ent 实体转换为服务模型。
func (r *tlsFingerprintProfileRepository) toModel(e *ent.TLSFingerprintProfile) *egress.TLSFingerprintProfile {
	p := &egress.TLSFingerprintProfile{
		ID:                      e.ID,
		Name:                    e.Name,
		Description:             e.Description,
		EnableGREASE:            e.EnableGrease,
		CipherSuites:            e.CipherSuites,
		Curves:                  e.Curves,
		PointFormats:            e.PointFormats,
		SignatureAlgorithms:     e.SignatureAlgorithms,
		SignatureAlgorithmsCert: e.SignatureAlgorithmsCert,
		ALPNProtocols:           e.AlpnProtocols,
		SupportedVersions:       e.SupportedVersions,
		KeyShareGroups:          e.KeyShareGroups,
		PSKModes:                e.PskModes,
		Extensions:              e.Extensions,
		CreatedAt:               e.CreatedAt,
		UpdatedAt:               e.UpdatedAt,
	}

	// 确保切片不为 nil
	if p.CipherSuites == nil {
		p.CipherSuites = []uint16{}
	}
	if p.Curves == nil {
		p.Curves = []uint16{}
	}
	if p.PointFormats == nil {
		p.PointFormats = []uint16{}
	}
	if p.SignatureAlgorithms == nil {
		p.SignatureAlgorithms = []uint16{}
	}
	if p.SignatureAlgorithmsCert == nil {
		p.SignatureAlgorithmsCert = []uint16{}
	}
	if p.ALPNProtocols == nil {
		p.ALPNProtocols = []string{}
	}
	if p.SupportedVersions == nil {
		p.SupportedVersions = []uint16{}
	}
	if p.KeyShareGroups == nil {
		p.KeyShareGroups = []uint16{}
	}
	if p.PSKModes == nil {
		p.PSKModes = []uint16{}
	}
	if p.Extensions == nil {
		p.Extensions = []uint16{}
	}

	return p
}
