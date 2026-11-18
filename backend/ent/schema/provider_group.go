package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProviderGroup 定义 provider_groups 关联表。
// 该表保存绑定创建时间，并使用联合主键。
type ProviderGroup struct {
	ent.Schema
}

func (ProviderGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "provider_groups"},
		// Composite primary key: (provider_id, group_id).
		field.ID("provider_id", "group_id"),
	}
}

func (ProviderGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id"),
		field.Int64("group_id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ProviderGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("provider", Provider.Type).
			Unique().
			Required().
			Field("provider_id"),
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id"),
	}
}

func (ProviderGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
	}
}
