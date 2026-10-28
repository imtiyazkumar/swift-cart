// Package document contains the ordered, canonical application schema.
package document

import _ "embed"

//go:embed 001_schema.sql
var schemaSQL string

//go:embed 002_constraints.sql
var constraintsSQL string

// BehaviorsSQL refreshes current trigger functions without replaying table DDL.
//
//go:embed 003_behaviors.sql
var BehaviorsSQL string

//go:embed 004_geography.sql
var geographySQL string

// SQL installs one final schema, with no historical intermediate definitions.
var SQL = schemaSQL + "\n" + constraintsSQL + "\n" + BehaviorsSQL + "\n" + geographySQL
