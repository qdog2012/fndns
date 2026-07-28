package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/fndns/manager/internal/model"
	"github.com/fndns/manager/internal/provider"
)

func TestRealProviderReadOnlyConnectivity(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	factory := provider.NewFactory(client)
	tests := []struct {
		name     string
		kind     string
		secret   model.CredentialSecret
		complete bool
	}{
		{name: "cloudflare", kind: model.ProviderCloudflare, secret: model.CredentialSecret{Token: os.Getenv("FNDNS_CLOUDFLARE_TOKEN")}, complete: os.Getenv("FNDNS_CLOUDFLARE_TOKEN") != ""},
		{name: "tencent", kind: model.ProviderTencent, secret: model.CredentialSecret{SecretID: os.Getenv("FNDNS_TENCENT_SECRET_ID"), SecretKey: os.Getenv("FNDNS_TENCENT_SECRET_KEY")}, complete: os.Getenv("FNDNS_TENCENT_SECRET_ID") != "" && os.Getenv("FNDNS_TENCENT_SECRET_KEY") != ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.complete {
				t.Skip("未提供本地环境变量")
			}
			remote, err := factory.Build(test.kind, test.secret)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			if err := remote.Validate(ctx); err != nil {
				t.Fatal(err)
			}
			domains, err := remote.ListDomains(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("read-only validation succeeded; domains=%d", len(domains))
		})
	}
}
