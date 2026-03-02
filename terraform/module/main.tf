locals {
  attrID       = "id"
  attrType     = "type"
  attrLabel    = "label"
  attrParentID = "parent_id"
}

resource "aws_dynamodb_table" "this" {
  name = var.table_name

  hash_key  = local.attrType
  range_key = local.attrID

  billing_mode   = "PROVISIONED"
  read_capacity  = var.read_capacity
  table_class    = "STANDARD"
  write_capacity = var.write_capacity

  attribute {
    name = local.attrType
    type = "S"
  }

  attribute {
    name = local.attrID
    type = "S"
  }

  attribute {
    name = local.attrParentID
    type = "S"
  }

  attribute {
    name = local.attrLabel
    type = "S"
  }

  global_secondary_index {
    name = "ByParentAndLabel"

    projection_type = "ALL"
    read_capacity   = var.read_capacity
    write_capacity  = var.write_capacity

    key_schema {
      key_type       = "HASH"
      attribute_name = local.attrParentID
    }

    key_schema {
      key_type       = "RANGE"
      attribute_name = local.attrLabel
    }
  }

  global_secondary_index {
    name = "ByType"

    projection_type = "ALL"
    read_capacity   = var.read_capacity
    write_capacity  = var.write_capacity

    key_schema {
      key_type       = "HASH"
      attribute_name = local.attrType
    }

  }

  local_secondary_index {
    name = "ByTypeAndLabel"

    range_key       = local.attrLabel
    projection_type = "ALL"
  }

  local_secondary_index {
    name = "ByTypeAndParent"

    range_key       = local.attrParentID
    projection_type = "ALL"
  }

  server_side_encryption {
    enabled     = true
    kms_key_arn = var.kms_key_arn
  }

  point_in_time_recovery {
    enabled = true
  }
}

resource "null_resource" "guardian" {
  count = var.enable_table_guardian ? 1 : 0

  triggers = {
    key_arn = aws_dynamodb_table.this.arn
  }

  lifecycle {
    prevent_destroy = true
  }
}
