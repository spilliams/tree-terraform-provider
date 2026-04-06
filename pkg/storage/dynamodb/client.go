package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/spilliams/tree-terraform-provider/internal/slug"
	"github.com/spilliams/tree-terraform-provider/pkg/storage"
)

type Client struct {
	region    string
	tableName string
	keyARN    string

	ddb *dynamodb.Client
}

func NewClient(ctx context.Context, profile, region, tableName, keyARN string) (storage.EntityStorer, error) {
	this := &Client{
		region:    region,
		tableName: tableName,
		keyARN:    keyARN,
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}
	this.ddb = dynamodb.NewFromConfig(cfg)

	err = this.createTableIfNotExists(ctx)
	if err != nil {
		return nil, err
	}

	return this, nil
}

const (
	storageKeyType = "type"
	storageKeyID   = "id"

	storageAttrParentID   = "parent_id"
	storageAttrLabel      = "label"
	storageAttrAttributes = "attributes"

	storageGSIByParentAndLabel = "ByParentAndLabel"
	storageGSIByType           = "ByType"

	storageLSIByTypeAndLabel  = "ByTypeAndLabel"
	storageLSIByTypeAndParent = "ByTypeAndParent"
)

func (client *Client) createTableIfNotExists(ctx context.Context) error {
	describeTableOutput, err := client.ddb.DescribeTable(ctx,
		&dynamodb.DescribeTableInput{
			TableName: aws.String(client.tableName),
		},
	)
	if err == nil {
		// table already exists
		if describeTableOutput != nil {
			tflog.Debug(ctx, fmt.Sprintf("table %s exists", client.tableName), map[string]interface{}{"tableID": *describeTableOutput.Table.TableId})
		}
		return nil
	}

	var respErr *smithyhttp.ResponseError
	if ok := errors.As(err, &respErr); ok && respErr.Response != nil {
		statusCode := respErr.Response.StatusCode
		if statusCode != http.StatusBadRequest {
			tflog.Warn(ctx, fmt.Sprintf("DescribeTable failed with HTTP status %d: %s", statusCode, err.Error()))
		}
	} else {
		tflog.Warn(ctx, fmt.Sprintf("unexpected error during DescribeTable: %s", err.Error()))
		return err
	}

	input := &dynamodb.CreateTableInput{
		TableName: aws.String(client.tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(storageKeyType),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String(storageKeyID),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String(storageAttrParentID),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String(storageAttrLabel),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(storageKeyType),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String(storageKeyID),
				KeyType:       types.KeyTypeRange,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(storageGSIByParentAndLabel),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(storageAttrParentID),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String(storageAttrLabel),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String(storageGSIByType),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(storageKeyType),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
		LocalSecondaryIndexes: []types.LocalSecondaryIndex{
			{
				IndexName: aws.String(storageLSIByTypeAndLabel),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(storageKeyType),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String(storageAttrLabel),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String(storageLSIByTypeAndParent),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(storageKeyType),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String(storageAttrParentID),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
		BillingMode: types.BillingModeProvisioned,
		SSESpecification: &types.SSESpecification{
			Enabled:        aws.Bool(true),
			SSEType:        types.SSETypeKms,
			KMSMasterKeyId: aws.String(client.keyARN),
		},
	}
	_, err = client.ddb.CreateTable(ctx, input)
	return err
}

var (
	ErrCannotDeleteEntity   = errors.New("cannot delete entity")
	ErrCollisionParentLabel = errors.New("an entity with that parent and label already exists")
	ErrCollisionTypeLabel   = errors.New("an entity with that type and label already exists")
	ErrNilQueryOutput       = errors.New("something went wrong: the query output was nil")
	ErrNotFoundEntity       = errors.New("entity not found")
	ErrTooManyFound         = errors.New("multiple exist where there must only be one")
)

func (client *Client) GetEntityByID(ctx context.Context, entityType, id string) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("GetEntityByID %q", id))
	output, err := client.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: id},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if output.Item == nil {
		return nil, fmt.Errorf("%w: %q", ErrNotFoundEntity, id)
	}
	return itemToEntity(output.Item)
}

func (client *Client) GetEntity(ctx context.Context, entityType, label string) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("GetEntity %q %q", entityType, label))
	output, err := client.ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(client.tableName),
		IndexName:              aws.String(storageLSIByTypeAndLabel),
		KeyConditionExpression: aws.String("#type = :type AND #label = :label"),
		ExpressionAttributeNames: map[string]string{
			"#type":  storageKeyType,
			"#label": storageAttrLabel,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type":  &types.AttributeValueMemberS{Value: entityType},
			":label": &types.AttributeValueMemberS{Value: label},
		},
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Items == nil {
		return nil, ErrNilQueryOutput
	}
	if len(output.Items) == 0 {
		return nil, fmt.Errorf("%w: type %q and label %q", ErrNotFoundEntity, entityType, label)
	}
	if len(output.Items) > 1 {
		return nil, fmt.Errorf("%w: type %q and label %q", ErrTooManyFound, entityType, label)
	}

	return itemToEntity(output.Items[0])
}

func (client *Client) CreateEntity(ctx context.Context, entityType, label string, attributes map[string]interface{}) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("CreateEntity %q %q", entityType, label))
	// make sure type+name doesn't collide
	output, err := client.ddb.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(client.tableName),
		IndexName: aws.String(storageLSIByTypeAndLabel),

		KeyConditionExpression: aws.String("#type = :type AND #label = :label"),
		ExpressionAttributeNames: map[string]string{
			"#type":  storageKeyType,
			"#label": storageAttrLabel,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type":  &types.AttributeValueMemberS{Value: entityType},
			":label": &types.AttributeValueMemberS{Value: label},
		},
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Items == nil {
		return nil, ErrNilQueryOutput
	}
	if len(output.Items) > 0 {
		return nil, ErrCollisionTypeLabel
	}

	id := slug.Generate(entityType)

	// create item as long as type+ID doesn't collide
	_, err = client.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(client.tableName),
		Item: map[string]types.AttributeValue{
			storageKeyType:        &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:          &types.AttributeValueMemberS{Value: id},
			storageAttrLabel:      &types.AttributeValueMemberS{Value: label},
			storageAttrAttributes: &types.AttributeValueMemberM{Value: attributesToMap(attributes)},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": storageKeyType,
			"#id":   storageKeyID,
		},
		ConditionExpression: aws.String("attribute_not_exists(#type) AND attribute_not_exists(#id)"),
	})
	if err != nil {
		return nil, err
	}

	return &entity{
		EntityType:       entityType,
		EntityID:         id,
		EntityLabel:      label,
		EntityAttributes: attributes,
	}, nil
}

func (client *Client) CreateChildEntity(ctx context.Context, entityType, label, parentType, parentID string, attributes map[string]interface{}) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("CreateChild %q %q %q %q", entityType, label, parentType, parentID))
	id := slug.Generate(entityType)
	object := &entity{
		EntityType:       entityType,
		EntityID:         id,
		EntityLabel:      label,
		EntityAttributes: attributes,
	}

	// make sure parent exists
	parent, err := client.GetEntityByID(ctx, parentType, parentID)
	if err != nil {
		return nil, err
	}

	object.EntityParentID = parent.ID()

	// make sure label is unique within the parent
	output, err := client.ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(client.tableName),
		IndexName:              aws.String(storageGSIByParentAndLabel),
		KeyConditionExpression: aws.String("#parent_id = :parent_id AND #label = :label"),
		ExpressionAttributeNames: map[string]string{
			"#parent_id": storageAttrParentID,
			"#label":     storageAttrLabel,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":parent_id": &types.AttributeValueMemberS{Value: parentID},
			":label":     &types.AttributeValueMemberS{Value: label},
		},
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Items == nil {
		return nil, ErrNilQueryOutput
	}
	if len(output.Items) > 0 {
		return nil, ErrCollisionParentLabel
	}

	_, err = client.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(client.tableName),
		Item: map[string]types.AttributeValue{
			storageKeyType:        &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:          &types.AttributeValueMemberS{Value: id},
			storageAttrLabel:      &types.AttributeValueMemberS{Value: label},
			storageAttrParentID:   &types.AttributeValueMemberS{Value: parentID},
			storageAttrAttributes: &types.AttributeValueMemberM{Value: attributesToMap(attributes)},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": storageKeyType,
			"#id":   storageKeyID,
		},
		ConditionExpression: aws.String("attribute_not_exists(#type) AND attribute_not_exists(#id)"),
	})
	if err != nil {
		return nil, err
	}

	return object, nil
}

func (client *Client) GetChildEntity(ctx context.Context, label, parentID string) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("GetChildEntity %q %q", label, parentID))
	output, err := client.ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(client.tableName),
		IndexName:              aws.String(storageGSIByParentAndLabel),
		KeyConditionExpression: aws.String("#parent_id = :parent_id AND #label = :label"),
		ExpressionAttributeNames: map[string]string{
			"#parent_id": storageAttrParentID,
			"#label":     storageAttrLabel,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":parent_id": &types.AttributeValueMemberS{Value: parentID},
			":label":     &types.AttributeValueMemberS{Value: label},
		},
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Items == nil {
		return nil, ErrNilQueryOutput
	}
	if len(output.Items) == 0 {
		return nil, fmt.Errorf("%w with parent ID %q and label %q", ErrNotFoundEntity, parentID, label)
	}
	if len(output.Items) > 1 {
		return nil, fmt.Errorf("%w: parent ID %q and label %q", ErrTooManyFound, parentID, label)
	}

	return itemToEntity(output.Items[0])
}

func (client *Client) ListEntities(ctx context.Context, entityType, labelFilter, parentIDFilter string) ([]storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("ListEntities %q %q %q", entityType, labelFilter, parentIDFilter))
	input := &dynamodb.QueryInput{
		TableName:              aws.String(client.tableName),
		IndexName:              aws.String(storageGSIByType),
		KeyConditionExpression: aws.String("#type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": storageKeyType,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: entityType},
		},
	}

	filterExprs := []string{}
	if labelFilter != "" {
		filterExprs = append(filterExprs, "contains(#label, :label)")
		input.ExpressionAttributeNames["#label"] = storageAttrLabel
		input.ExpressionAttributeValues[":label"] = &types.AttributeValueMemberS{Value: labelFilter}
	}
	if parentIDFilter != "" {
		filterExprs = append(filterExprs, "#parent_id = :parent_id")
		input.ExpressionAttributeNames["#parent_id"] = storageAttrParentID
		input.ExpressionAttributeValues[":parent_id"] = &types.AttributeValueMemberS{Value: parentIDFilter}
	}
	if len(filterExprs) > 0 {
		input.FilterExpression = aws.String(strings.Join(filterExprs, " AND "))
	}

	output, err := client.ddb.Query(ctx, input)
	if err != nil {
		return nil, err
	}
	if output == nil || output.Items == nil {
		return nil, ErrNilQueryOutput
	}
	entities := make([]storage.Entity, len(output.Items))
	for i, item := range output.Items {
		entities[i], err = itemToEntity(item)
		if err != nil {
			return nil, err
		}
	}
	return entities, nil
}

func (client *Client) UpdateEntity(ctx context.Context, entityType, id, newLabel string) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("UpdatEntity %q %q %q", entityType, id, newLabel))
	// ensure new label is available
	this, err := client.GetEntityByID(ctx, entityType, id)
	if err != nil {
		return nil, err
	}
	_, err = client.GetChildEntity(ctx, newLabel, this.ParentID())
	if err == nil {
		return nil, ErrCollisionParentLabel
	}
	if !errors.Is(err, ErrNotFoundEntity) {
		return nil, err
	}

	output, err := client.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression: aws.String("SET #label = :new_label"),
		ExpressionAttributeNames: map[string]string{
			"#label": storageAttrLabel,
			"#type":  storageKeyType,
			"#id":    storageKeyID,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_label": &types.AttributeValueMemberS{Value: newLabel},
		},
		ConditionExpression: aws.String("attribute_not_exists(#type) AND attribute_not_exists(#id)"),
		ReturnValues:        types.ReturnValueAllNew,
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Attributes == nil {
		return nil, ErrNilQueryOutput
	}
	return itemToEntity(output.Attributes)
}

func (client *Client) UpdateChildEntity(ctx context.Context, childType, childID, newChildLabel, parentType, newParentID string) (storage.Entity, error) {
	tflog.Debug(ctx, fmt.Sprintf("UpdateChildEntity %q %q %q %q %q", childType, childID, newChildLabel, parentType, newParentID))
	// ensure new parent exists
	_, err := client.GetEntityByID(ctx, parentType, newParentID)
	if err != nil {
		return nil, err
	}

	// ensure new label is available
	_, err = client.GetChildEntity(ctx, newChildLabel, newParentID)
	if err == nil {
		return nil, ErrCollisionParentLabel
	}
	if !errors.Is(err, ErrNotFoundEntity) {
		return nil, err
	}

	// update the item
	output, err := client.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: childType},
			storageKeyID:   &types.AttributeValueMemberS{Value: childID},
		},
		UpdateExpression: aws.String("SET #label = :new_label, #parent_id = :new_parent_id"),
		ExpressionAttributeNames: map[string]string{
			"#label":     storageAttrLabel,
			"#parent_id": storageAttrParentID,
			"#type":      storageKeyType,
			"#id":        storageKeyID,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_label":     &types.AttributeValueMemberS{Value: newChildLabel},
			":new_parent_id": &types.AttributeValueMemberS{Value: newParentID},
		},
		ConditionExpression: aws.String("attribute_not_exists(#type) AND attribute_not_exists(#id)"),
		ReturnValues:        types.ReturnValueAllNew,
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Attributes == nil {
		return nil, ErrNilQueryOutput
	}
	return itemToEntity(output.Attributes)
}

func (client *Client) UpdateAttribute(ctx context.Context, entityType, entityID, attributeName string, attributeValue interface{}) error {
	tflog.Debug(ctx, fmt.Sprintf("UpdateAttribute %q %q %q %q", entityType, entityID, attributeName, attributeValue))

	value := ifaceToAttributeValue(attributeValue)

	_, err := client.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: entityID},
		},
		UpdateExpression: aws.String("SET #attributes.#key = :value"),
		ExpressionAttributeNames: map[string]string{
			"#attributes": storageAttrAttributes,
			"#key":        attributeName,
			"#type":       storageKeyType,
			"#id":         storageKeyID,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":value": value,
		},
		ConditionExpression: aws.String("attribute_exists(#type) AND attribute_exists(#id)"),
	})
	return err
}

func (client *Client) UpdateAttributes(ctx context.Context, entityType, entityID string, attributes map[string]interface{}) error {
	tflog.Debug(ctx, fmt.Sprintf("UpdateAttributes %q %q", entityType, entityID))
	_, err := client.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: entityID},
		},
		UpdateExpression: aws.String("SET #attributes = :new_attributes"),
		ExpressionAttributeNames: map[string]string{
			"#attributes": storageAttrAttributes,
			"#type":       storageKeyType,
			"#id":         storageKeyID,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_attributes": &types.AttributeValueMemberM{Value: attributesToMap(attributes)},
		},
		ConditionExpression: aws.String("attribute_exists(#type) AND attribute_exists(#id)"),
	})
	return err
}

func (client *Client) DeleteAttribute(ctx context.Context, entityType, entityID, attributeName string) error {
	tflog.Debug(ctx, fmt.Sprintf("DeleteAttribute %q %q %q", entityType, entityID, attributeName))

	_, err := client.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: entityID},
		},
		UpdateExpression: aws.String("REMOVE #attributes.#key"),
		ExpressionAttributeNames: map[string]string{
			"#attributes": storageAttrAttributes,
			"#key":        attributeName,
			"#type":       storageKeyType,
			"#id":         storageKeyID,
		},
		ConditionExpression: aws.String("attribute_exists(#type) AND attribute_exists(#id)"),
	})
	return err
}

func (client *Client) DeleteEntity(ctx context.Context, entityType, childType, id string) error {
	tflog.Debug(ctx, fmt.Sprintf("DeleteEntity %q %q %q", entityType, childType, id))
	// ensure this entity does not have any children
	if len(childType) > 0 {
		output, err := client.ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(client.tableName),
			IndexName:              aws.String(storageLSIByTypeAndParent),
			KeyConditionExpression: aws.String("#type = :type AND #parent_id = :parent_id"),
			ExpressionAttributeNames: map[string]string{
				"#type":      storageKeyType,
				"#parent_id": storageAttrParentID,
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":type":      &types.AttributeValueMemberS{Value: childType},
				":parent_id": &types.AttributeValueMemberS{Value: id},
			},
		})
		if err != nil {
			return err
		}
		if output == nil || output.Items == nil {
			return ErrNilQueryOutput
		}
		if len(output.Items) > 0 {
			return fmt.Errorf("%s %s has children: %w", entityType, id, ErrCannotDeleteEntity)
		}
	}

	_, err := client.ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(client.tableName),
		Key: map[string]types.AttributeValue{
			storageKeyType: &types.AttributeValueMemberS{Value: entityType},
			storageKeyID:   &types.AttributeValueMemberS{Value: id},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": storageKeyType,
			"#id":   storageKeyID,
		},
		ConditionExpression: aws.String("attribute_exists(#type) and attribute_exists(#id)"),
	})
	return err
}
