package memory_test

import (
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestSecretsContract(t *testing.T) {
	storetest.SecretStore(t, func(*testing.T) ports.SecretStore { return memory.NewSecrets() })
}
