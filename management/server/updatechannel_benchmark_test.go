package server

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	nbdns "github.com/netbirdio/netbird/dns"
	nbgroup "github.com/netbirdio/netbird/management/server/group"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	route2 "github.com/netbirdio/netbird/route"
)

func initTestAccount(b *testing.B, numPerAccount int) *Account {
	b.Helper()

	account := newAccountWithId(context.Background(), "account_id", "testuser", "")
	groupALL, err := account.GetGroupAll()
	if err != nil {
		b.Fatal(err)
	}
	setupKey, _ := GenerateDefaultSetupKey()
	account.SetupKeys[setupKey.Key] = setupKey
	for n := 0; n < numPerAccount; n++ {
		netIP := randomIPv4()
		peerID := fmt.Sprintf("%s-peer-%d", account.Id, n)

		peer := &nbpeer.Peer{
			ID:         peerID,
			Key:        peerID,
			IP:         netIP,
			Name:       peerID,
			DNSLabel:   peerID,
			UserID:     userID,
			Status:     &nbpeer.PeerStatus{Connected: false, LastSeen: time.Now()},
			SSHEnabled: false,
		}
		account.Peers[peerID] = peer
		group, _ := account.GetGroupAll()
		group.Peers = append(group.Peers, peerID)
		user := &User{
			Id:        fmt.Sprintf("%s-user-%d", account.Id, n),
			AccountID: account.Id,
		}
		account.Users[user.Id] = user
		route := &route2.Route{
			ID:          route2.ID(fmt.Sprintf("network-id-%d", n)),
			Description: "base route",
			NetID:       route2.NetID(fmt.Sprintf("network-id-%d", n)),
			Network:     netip.MustParsePrefix(netIP.String() + "/24"),
			NetworkType: route2.IPv4Network,
			Metric:      9999,
			Masquerade:  false,
			Enabled:     true,
			Groups:      []string{groupALL.ID},
		}
		account.Routes[route.ID] = route

		group = &nbgroup.Group{
			ID:        fmt.Sprintf("group-id-%d", n),
			AccountID: account.Id,
			Name:      fmt.Sprintf("group-id-%d", n),
			Issued:    "api",
			Peers:     nil,
		}
		account.Groups[group.ID] = group

		nameserver := &nbdns.NameServerGroup{
			ID:                   fmt.Sprintf("nameserver-id-%d", n),
			AccountID:            account.Id,
			Name:                 fmt.Sprintf("nameserver-id-%d", n),
			Description:          "",
			NameServers:          []nbdns.NameServer{{IP: netip.MustParseAddr(netIP.String()), NSType: nbdns.UDPNameServerType}},
			Groups:               []string{group.ID},
			Primary:              false,
			Domains:              nil,
			Enabled:              false,
			SearchDomainsEnabled: false,
		}
		account.NameServerGroups[nameserver.ID] = nameserver

		setupKey, _ := GenerateDefaultSetupKey()
		account.SetupKeys[setupKey.Key] = setupKey
	}

	group := &nbgroup.Group{
		ID:        "randomID",
		AccountID: account.Id,
		Name:      "randomName",
		Issued:    "api",
		Peers:     groupALL.Peers[:numPerAccount-1],
	}
	account.Groups[group.ID] = group

	account.Policies = []*Policy{
		{
			ID:          "RuleDefault",
			Name:        "Default",
			Description: "This is a default rule that allows connections between all the resources",
			Enabled:     true,
			Rules: []*PolicyRule{
				{
					ID:            "RuleDefault",
					Name:          "Default",
					Description:   "This is a default rule that allows connections between all the resources",
					Bidirectional: true,
					Enabled:       true,
					Protocol:      PolicyRuleProtocolTCP,
					Action:        PolicyTrafficActionAccept,
					Sources: []string{
						group.ID,
					},
					Destinations: []string{
						group.ID,
					},
				},
				{
					ID:            "RuleDefault2",
					Name:          "Default",
					Description:   "This is a default rule that allows connections between all the resources",
					Bidirectional: true,
					Enabled:       true,
					Protocol:      PolicyRuleProtocolUDP,
					Action:        PolicyTrafficActionAccept,
					Sources: []string{
						groupALL.ID,
					},
					Destinations: []string{
						groupALL.ID,
					},
				},
			},
		},
	}
	return account
}

// 1000 - 6717416375 ns/op
// 500 -  1732888875 ns/op
func BenchmarkTest_updateAccountPeers100(b *testing.B) {
	dataDir := b.TempDir()
	store, cleanUp, err := NewTestStoreFromSQL(context.Background(), "", dataDir)
	b.Cleanup(cleanUp)

	um := NewPeersUpdateManager(nil)
	am, err := BuildManager(context.Background(), store, um, nil, "", "netbird.selfhosted", nil, nil, false, MocIntegratedValidator{}, nil)
	if err != nil {
		b.Fatal(err)
	}

	account := initTestAccount(b, 100)

	err = store.SaveAccount(context.Background(), account)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		am.updateAccountPeers(context.Background(), account)
	}
}
