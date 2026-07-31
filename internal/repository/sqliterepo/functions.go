package sqliterepo

import (
	"database/sql/driver"
	"fmt"
	"strings"

	sqlite "modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(
		"ooshdaq_unicode_lower",
		1,
		func(_ *sqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			if len(arguments) != 1 {
				return nil, fmt.Errorf("ooshdaq_unicode_lower expects one argument")
			}
			switch value := arguments[0].(type) {
			case nil:
				return nil, nil
			case string:
				return strings.ToLower(value), nil
			case []byte:
				return strings.ToLower(string(value)), nil
			default:
				return nil, fmt.Errorf("ooshdaq_unicode_lower expects text")
			}
		},
	)
}
