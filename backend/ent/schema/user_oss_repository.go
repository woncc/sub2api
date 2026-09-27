package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserOSSRepository is a per-user object storage repository.
// Runtime access uses the SQL repository; this schema documents the table.
type UserOSSRepository struct {
	ent.Schema
}

func (UserOSSRepository) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_oss_repositories"},
	}
}

func (UserOSSRepository) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserOSSRepository) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("Owning user. SQL migration cascades deletes with the user."),
		field.String("provider").
			MaxLen(32).
			NotEmpty(),
		field.String("bucket").
			MaxLen(512).
			NotEmpty(),
		field.String("domain").
			MaxLen(512).
			Default(""),
		field.String("region").
			MaxLen(128).
			Default(""),
		field.String("endpoint").
			MaxLen(512).
			Default(""),
		field.String("access_key_id").
			MaxLen(256).
			Default(""),
		field.String("secret_encrypted").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("account_id").
			MaxLen(128).
			Default(""),
		field.Bool("force_path_style").
			Default(false),
		field.String("bucket_url").
			MaxLen(512).
			Default(""),
	}
}

func (UserOSSRepository) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
