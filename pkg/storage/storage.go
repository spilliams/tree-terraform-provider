package storage

import "context"

type Entity interface {
	Type() string
	ID() string
	Label() string
	ParentID() string
	Attributes() map[string]interface{}
}

type EntityStorer interface {
	GetEntityByID(ctx context.Context, entityType, entityID string) (Entity, error)
	GetEntity(ctx context.Context, entityType, entityLabel string) (Entity, error)
	CreateEntity(ctx context.Context, entityType, entityLabel string, attributes map[string]interface{}) (Entity, error)
	CreateChildEntity(ctx context.Context, entityType, entityLabel, parentType, parentID string, attributes map[string]interface{}) (Entity, error)
	GetChildEntity(ctx context.Context, childLabel, parentID string) (Entity, error)
	ListEntities(ctx context.Context, entityType, labelFilter, parentIDFilter string) ([]Entity, error)
	UpdateEntity(ctx context.Context, entityType, entityID, newLabel string) (Entity, error)
	UpdateChildEntity(ctx context.Context, childType, childID, newChildLabel, parentType, newParentID string) (Entity, error)
	UpdateAttribute(ctx context.Context, entityType, entityID, attributeName string, attributeValue interface{}) error
	UpdateAttributes(ctx context.Context, entityType, entityID string, attributes map[string]interface{}) error
	DeleteEntity(ctx context.Context, entityType, childType, entityID string) error
}
