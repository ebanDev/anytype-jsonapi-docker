package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	defaultGrpcAddr   = "127.0.0.1:31007"
	defaultJsonApi    = "127.0.0.1:31009"
	defaultAppName    = "jsonapi-cli"
	defaultProfile    = "Json API user"
	defaultPlatform   = "jsonapi-cli"
	defaultVersion    = "0.0.0-jsonapi"
	defaultTimeout    = 2 * time.Minute
	defaultWaitSpaces = 2 * time.Minute
	createProfileIcon = int32(0)
)

func main() {
	var (
		rootPath     string
		accountID    string
		mnemonic     string
		accountKey   string
		accountName  string
		grpcAddr     string
		jsonapiAddr  string
		appName      string
		createNewAcc bool
		platform     string
		version      string
		timeout      time.Duration
		waitSpaces   time.Duration
	)

	flag.StringVar(&rootPath, "root", "", "Root path where the account data is stored (required)")
	flag.StringVar(&accountID, "account-id", "", "Account ID (optional, derived from mnemonic/accountKey if empty)")
	flag.StringVar(&mnemonic, "mnemonic", "", "Mnemonic to recover an existing wallet")
	flag.StringVar(&accountKey, "account-key", "", "Account master key (base64) to recover an existing wallet")
	flag.StringVar(&accountName, "name", defaultProfile, "Profile name when creating a new account")
	flag.StringVar(&grpcAddr, "grpc", defaultGrpcAddr, "gRPC address of the running anytype-heart server")
	flag.StringVar(&jsonapiAddr, "jsonapi", defaultJsonApi, "Listen address for the JsonAPI (passed to AccountCreate/AccountSelect)")
	flag.StringVar(&appName, "app-name", defaultAppName, "App name for generating an API key")
	flag.StringVar(&platform, "platform", defaultPlatform, "Client platform label for InitialSetParameters")
	flag.StringVar(&version, "version", defaultVersion, "Client version label for InitialSetParameters")
	flag.DurationVar(&timeout, "timeout", defaultTimeout, "Per-RPC timeout (e.g. 90s, 2m)")
	flag.DurationVar(&waitSpaces, "wait-spaces", defaultWaitSpaces, "Wait up to this duration for spaces to sync before listing")
	flag.BoolVar(&createNewAcc, "create", false, "Create a fresh account instead of selecting an existing one")
	flag.Parse()

	if rootPath == "" {
		log.Fatalf("missing required flag: -root")
	}

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial gRPC: %v", err)
	}
	defer conn.Close()

	client := service.NewClientCommandsClient(conn)

	sendInitialSetParameters(ctx, client, platform, version, rootPath, timeout)

	bootstrap := func() (string, string) {
		if createNewAcc {
			mnemonic, accountKey = handleWalletCreate(ctx, client, rootPath, timeout)
			accountID = handleAccountCreate(ctx, client, rootPath, accountName, jsonapiAddr, timeout)
		} else {
			if mnemonic == "" && accountKey == "" {
				log.Fatalf("provide either -mnemonic or -account-key (or use -create)")
			}
			handleWalletRecover(ctx, client, rootPath, mnemonic, accountKey, timeout)
			if accountID == "" {
				accountID = deriveAccountID(mnemonic, accountKey)
			}
			handleAccountSelect(ctx, client, accountID, rootPath, jsonapiAddr, timeout)
		}
		sessionToken := createFullSession(ctx, client, mnemonic, accountKey, timeout)
		appKey := handleCreateApp(ctx, client, appName, timeout, sessionToken)
		return appKey, sessionToken
	}

	appKey, sessionToken := bootstrap()

	fmt.Println("----- JsonAPI is ready -----")
	fmt.Printf("Account ID: %s\n", accountID)
	fmt.Printf("JsonAPI listen address: http://%s\n", jsonapiAddr)
	fmt.Printf("Bearer token (app key): %s\n", appKey)
	fmt.Printf("Example: curl -H 'Authorization: Bearer %s' http://%s/v1/spaces\n", appKey, jsonapiAddr)

	if waitSpaces > 0 {
		if spaces, err := waitAndListSpaces(jsonapiAddr, appKey, waitSpaces); err == nil {
			printSpaces(spaces)
		} else {
			fmt.Printf("Spaces not ready within %s, retrying restart once...\n", waitSpaces)
			// restart the account to re-trigger sync
			handleAccountStop(ctx, client, timeout, sessionToken)
			// AccountSelect already includes JsonApiListenAddr
			handleAccountSelect(ctx, client, accountID, rootPath, jsonapiAddr, timeout)
			sessionToken = createFullSession(ctx, client, mnemonic, accountKey, timeout)
			appKey = handleCreateApp(ctx, client, appName, timeout, sessionToken)
			if spaces, err := waitAndListSpaces(jsonapiAddr, appKey, waitSpaces); err == nil {
				printSpaces(spaces)
			} else {
				fmt.Printf("Spaces still empty after retry (%s): %v\n", waitSpaces, err)
			}
		}
	}
}

func handleWalletCreate(ctx context.Context, client service.ClientCommandsClient, rootPath string, timeout time.Duration) (mnemonic string, accountKey string) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := client.WalletCreate(ctx, &pb.RpcWalletCreateRequest{RootPath: rootPath})
	checkPBError("WalletCreate", err, resp.GetError())

	fmt.Println("Wallet created.")
	fmt.Printf("Mnemonic: %s\n", resp.Mnemonic)
	fmt.Printf("Account key (base64): %s\n", resp.AccountKey)
	return resp.Mnemonic, resp.AccountKey
}

func handleWalletRecover(ctx context.Context, client service.ClientCommandsClient, rootPath, mnemonic, accountKey string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := client.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{
		RootPath: rootPath,
		Mnemonic:   mnemonic,
		AccountKey: accountKey,
	})
	checkPBError("WalletRecover", err, resp.GetError())
	fmt.Println("Wallet recovery completed.")
}

func handleAccountSelect(ctx context.Context, client service.ClientCommandsClient, accountID, rootPath, jsonapiAddr string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := client.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
		Id:               accountID,
		RootPath:         rootPath,
		JsonApiListenAddr: jsonapiAddr,
	})
	checkPBError("AccountSelect", err, resp.GetError())
	fmt.Printf("Account selected: %s\n", accountID)
}

func handleAccountCreate(ctx context.Context, client service.ClientCommandsClient, rootPath, name, jsonapiAddr string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := client.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
		Name:              name,
		Icon:              int64(createProfileIcon),
		JsonApiListenAddr: jsonapiAddr,
	})
	checkPBError("AccountCreate", err, resp.GetError())

	if resp.Account == nil {
		log.Fatalf("AccountCreate succeeded but account is nil")
	}
	fmt.Printf("Account created: %s\n", resp.Account.Id)
	return resp.Account.Id
}

func handleCreateApp(ctx context.Context, client service.ClientCommandsClient, appName string, timeout time.Duration, sessionToken string) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("token", sessionToken))

	resp, err := client.AccountLocalLinkCreateApp(ctx, &pb.RpcAccountLocalLinkCreateAppRequest{
		App: &model.AccountAuthAppInfo{
			AppName: appName,
			Scope:   model.AccountAuth_JsonAPI,
		},
	})
	checkPBError("AccountLocalLinkCreateApp", err, resp.GetError())

	if resp.AppKey == "" {
		log.Fatalf("AccountLocalLinkCreateApp returned empty app key")
	}
	return resp.AppKey
}

func deriveAccountID(mnemonic, accountKey string) string {
	if mnemonic != "" {
		derivation, err := core.WalletAccountAt(mnemonic, 0)
		if err != nil {
			log.Fatalf("derive account ID from mnemonic: %v", err)
		}
		return derivation.Identity.GetPublic().Account()
	}
	derivation, err := core.WalletDeriveFromAccountMasterNode(accountKey)
	if err != nil {
		log.Fatalf("derive account ID from account key: %v", err)
	}
	return derivation.Identity.GetPublic().Account()
}

func exitOnRPCError(prefix string, err error, rpcErr any) {
	checkPBError(prefix, err, rpcErr)
}

func sendInitialSetParameters(ctx context.Context, client service.ClientCommandsClient, platform, version, workdir string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
		Platform: platform,
		Version:  version,
		Workdir:  workdir,
	})
	checkPBError("InitialSetParameters", err, resp.GetError())
}

// checkPBError handles both transport errors and pb error payloads.
func checkPBError(prefix string, err error, pbErr any) {
	if err != nil {
		log.Fatalf("%s call failed: %v", prefix, err)
	}

	code, desc, ok := extractPBError(pbErr)
	if ok && code != 0 {
		log.Fatalf("%s RPC error: code=%d desc=%s", prefix, code, desc)
	}
}

func extractPBError(pbErr any) (int32, string, bool) {
	switch e := pbErr.(type) {
	case *pb.RpcWalletCreateResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	case *pb.RpcWalletRecoverResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	case *pb.RpcAccountSelectResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	case *pb.RpcAccountCreateResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	case *pb.RpcAccountLocalLinkCreateAppResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	case *pb.RpcInitialSetParametersResponseError:
		return int32(e.GetCode()), e.GetDescription(), true
	default:
		return 0, "", false
	}
}

func createFullSession(ctx context.Context, client service.ClientCommandsClient, mnemonic, accountKey string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := &pb.RpcWalletCreateSessionRequest{}
	switch {
	case accountKey != "":
		req.Auth = &pb.RpcWalletCreateSessionRequestAuthOfAccountKey{AccountKey: accountKey}
	case mnemonic != "":
		req.Auth = &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{Mnemonic: mnemonic}
	default:
		log.Fatalf("cannot create session: no mnemonic or accountKey available")
	}

	resp, err := client.WalletCreateSession(ctx, req)
	checkPBError("WalletCreateSession", err, resp.GetError())
	if resp.Token == "" {
		log.Fatalf("WalletCreateSession returned empty token")
	}
	return resp.Token
}

// Space holds a minimal view of the /v1/spaces response.
type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type spacesResponse struct {
	Data []Space `json:"data"`
}

func waitAndListSpaces(jsonapiAddr, appKey string, wait time.Duration) ([]Space, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(wait)
	url := fmt.Sprintf("http://%s/v1/spaces", jsonapiAddr)

	for {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+appKey)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var sr spacesResponse
			if err := json.NewDecoder(resp.Body).Decode(&sr); err == nil && len(sr.Data) > 0 {
				resp.Body.Close()
				return sr.Data, nil
			}
			resp.Body.Close()
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("spaces still empty or unavailable")
		}
		time.Sleep(5 * time.Second)
	}
}

func printSpaces(spaces []Space) {
	fmt.Printf("Spaces (%d):\n", len(spaces))
	for _, s := range spaces {
		fmt.Printf("- %s (%s)\n", s.Name, s.ID)
	}
}

func handleAccountStop(ctx context.Context, client service.ClientCommandsClient, timeout time.Duration, sessionToken string) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if sessionToken != "" {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("token", sessionToken))
	}

	resp, err := client.AccountStop(ctx, &pb.RpcAccountStopRequest{})
	checkPBError("AccountStop", err, resp.GetError())
}
