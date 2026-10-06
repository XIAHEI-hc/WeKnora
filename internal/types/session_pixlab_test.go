package types

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestSessionPixLabProjectCodeUsesMigrationColumnName(t *testing.T) {
	parsed, err := schema.Parse(&Session{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	field := parsed.LookUpField("PixLabProjectCode")
	require.NotNil(t, field)
	require.Equal(t, "pixlab_project_code", field.DBName)
}
