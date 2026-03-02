package dynamodb

import (
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type entity struct {
	EntityType       string                 `dynamodbav:"type"`
	EntityID         string                 `dynamodbav:"id"`
	EntityLabel      string                 `dynamodbav:"label"`
	EntityParentID   string                 `dynamodbav:"parent_id"`
	EntityAttributes map[string]interface{} `dynamodbav:"attributes"`
}

func itemToEntity(item map[string]types.AttributeValue) (*entity, error) {
	var r entity
	err := attributevalue.UnmarshalMap(item, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func ifaceToAttributeValue(in interface{}) types.AttributeValue {
	var out types.AttributeValue
	if vString, isString := in.(string); isString {
		out = &types.AttributeValueMemberS{Value: vString}
	}
	if vStringList, isStringList := in.([]string); isStringList {
		out = &types.AttributeValueMemberSS{Value: vStringList}
	}
	return out
}

func attributesToMap(attributes map[string]interface{}) map[string]types.AttributeValue {
	awsmap := make(map[string]types.AttributeValue)
	for k, v := range attributes {
		awsmap[k] = ifaceToAttributeValue(v)
	}
	return awsmap
}

func (r *entity) Type() string                       { return r.EntityType }
func (r *entity) ID() string                         { return r.EntityID }
func (r *entity) Label() string                      { return r.EntityLabel }
func (r *entity) ParentID() string                   { return r.EntityParentID }
func (r *entity) Attributes() map[string]interface{} { return r.EntityAttributes }
