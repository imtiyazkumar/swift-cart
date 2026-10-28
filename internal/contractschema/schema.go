// Package contractschema defines the database contract extracted from the product
// documents. It is isolated from historical migrations while the runtime cutover
// is developed, so an incomplete cutover cannot alter an existing installation.
package contractschema

import (
	_ "embed"
	"github.com/imtiyazkumar/yember-core/migrations/document"
)

//go:embed import_finance.sql
var importFinanceSQL string

var SQL = document.SQL

//go:embed contract.json
var Contract []byte
