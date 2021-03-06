package vault

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/hashicorp/faas-nomad/types"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
)

type VaultService struct {
	Client *api.Client
	Config *types.VaultConfig
	logger hclog.Logger
}

func NewVaultService(config *types.VaultConfig, log hclog.Logger) *VaultService {

	clientConfig := api.DefaultConfig()
	clientConfig.ConfigureTLS(&api.TLSConfig{Insecure: config.TLSSkipVerify})
	vaultClient, _ := api.NewClient(clientConfig)

	vaultClient.SetAddress(config.Addr)

	vs := &VaultService{
		Client: vaultClient,
		Config: config,
		logger: log.Named("vault_service"),
	}

	return vs
}

// Gets and sets the initial access token from Vault
func (vs *VaultService) Login() (api.Secret, error) {

	var vaultLogin api.Secret

	lResp, lErr := vs.DoRequest("POST", "/v1/auth/approle/login",
		map[string]interface{}{"role_id": vs.Config.AppRoleID, "secret_id": vs.Config.AppSecretID})

	if lErr != nil {
		return vaultLogin, lErr
	}

	if lResp.StatusCode != http.StatusOK {
		return vaultLogin, fmt.Errorf("Vault response status code %v", lResp.StatusCode)
	}

	lBody, _ := ioutil.ReadAll(lResp.Body)
	parseErr := json.Unmarshal(lBody, &vaultLogin)
	if parseErr != nil {
		return vaultLogin, parseErr
	}

	if vaultLogin.Auth != nil && len(vaultLogin.Auth.ClientToken) > 0 {
		vs.Client.SetToken(vaultLogin.Auth.ClientToken)

		r, renewErr := vs.Client.NewRenewer(&api.RenewerInput{
			Secret: &vaultLogin,
		})
		if renewErr != nil {
			vs.logger.Error(renewErr.Error())
		}
		go r.Renew()
	}

	return vaultLogin, nil
}

// Execute request to the configured Vault server
func (vs *VaultService) DoRequest(method string, path string, body interface{}) (*http.Response, error) {

	client := &http.Client{}
	trIgnore := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	createRequest := vs.Client.NewRequest(method, path)
	createRequest.SetJSONBody(body)

	request, _ := createRequest.ToHTTP()
	if vs.Config.TLSSkipVerify {
		client.Transport = trIgnore
	}
	return client.Do(request)
}
