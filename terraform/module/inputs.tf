variable "enable_table_guardian" {
  type    = bool
  default = true
}

variable "kms_key_arn" {
  type = string
}

variable "read_capacity" {
  type    = number
  default = 5
}

variable "table_name" {
  type = string
}

variable "write_capacity" {
  type    = number
  default = 2
}
