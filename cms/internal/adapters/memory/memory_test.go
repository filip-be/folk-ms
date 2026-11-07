package memory_test

import (
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestContract(t *testing.T) {
	storetest.DocumentStore(t, func(*testing.T) ports.DocumentStore { return memory.NewDocumentStore() })
	storetest.PublicStore(t, func(*testing.T) ports.PublicStore { return memory.NewPublicStore("http://x/pub") })
	storetest.AppendLog(t, func(*testing.T) ports.AppendLog { return memory.NewAppendLog() })
}
