package server

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nbdns "github.com/netbirdio/netbird/dns"
	"github.com/netbirdio/netbird/management/server/groups"
	"github.com/netbirdio/netbird/management/server/networks"
	"github.com/netbirdio/netbird/management/server/networks/resources"
	"github.com/netbirdio/netbird/management/server/networks/routers"
	routerTypes "github.com/netbirdio/netbird/management/server/networks/routers/types"
	networkTypes "github.com/netbirdio/netbird/management/server/networks/types"
	"github.com/netbirdio/netbird/management/server/permissions"
	"github.com/netbirdio/netbird/management/server/status"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/route"
)

const (
	groupAdminUserID = "testingAdminUser"
)

func TestDefaultAccountManager_CreateGroup(t *testing.T) {
	am, err := createManager(t)
	if err != nil {
		t.Error("failed to create account manager")
	}

	_, account, err := initTestGroupAccount(am)
	if err != nil {
		t.Fatalf("failed to init testing account: %s", err)
	}
	for _, group := range account.Groups {
		group.Issued = types.GroupIssuedIntegration
		err = am.SaveGroup(context.Background(), account.Id, groupAdminUserID, group, true)
		if err != nil {
			t.Errorf("should allow to create %s groups", types.GroupIssuedIntegration)
		}
	}

	for _, group := range account.Groups {
		group.Issued = types.GroupIssuedJWT
		err = am.SaveGroup(context.Background(), account.Id, groupAdminUserID, group, true)
		if err != nil {
			t.Errorf("should allow to create %s groups", types.GroupIssuedJWT)
		}
	}
	for _, group := range account.Groups {
		group.Issued = types.GroupIssuedAPI
		group.ID = ""
		err = am.SaveGroup(context.Background(), account.Id, groupAdminUserID, group, true)
		if err == nil {
			t.Errorf("should not create api group with the same name, %s", group.Name)
		}
	}
}

func TestDefaultAccountManager_DeleteGroup(t *testing.T) {
	am, err := createManager(t)
	if err != nil {
		t.Fatalf("failed to create account manager: %s", err)
	}

	_, account, err := initTestGroupAccount(am)
	if err != nil {
		t.Fatalf("failed to init testing account: %s", err)
	}

	testCases := []struct {
		name           string
		groupID        string
		expectedReason string
	}{
		{
			"route",
			"grp-for-route",
			"route",
		},
		{
			"route with peer groups",
			"grp-for-route2",
			"route",
		},
		{
			"name server groups",
			"grp-for-name-server-grp",
			"name server groups",
		},
		{
			"policy",
			"grp-for-policies",
			"policy",
		},
		{
			"setup keys",
			"grp-for-keys",
			"setup key",
		},
		{
			"users",
			"grp-for-users",
			"user",
		},
		{
			"integration",
			"grp-for-integration",
			"only service users with admin power can delete integration group",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err = am.DeleteGroup(context.Background(), account.Id, groupAdminUserID, testCase.groupID)
			if err == nil {
				t.Errorf("delete %s group successfully", testCase.groupID)
				return
			}

			var sErr *status.Error
			if errors.As(err, &sErr) {
				if sErr.Message != testCase.expectedReason {
					t.Errorf("invalid error case: %s, expected: %s", sErr.Message, testCase.expectedReason)
				}
				return
			}

			var gErr *GroupLinkError
			ok := errors.As(err, &gErr)
			if !ok {
				t.Error("invalid error type")
				return
			}
			if gErr.Resource != testCase.expectedReason {
				t.Errorf("invalid error case: %s, expected: %s", gErr.Resource, testCase.expectedReason)
			}
		})
	}
}

func TestDefaultAccountManager_DeleteGroups(t *testing.T) {
	am, err := createManager(t)
	assert.NoError(t, err, "Failed to create account manager")

	manager, account, err := initTestGroupAccount(am)
	assert.NoError(t, err, "Failed to init testing account")

	groups := make([]*types.Group, 10)
	for i := 0; i < 10; i++ {
		groups[i] = &types.Group{
			ID:        fmt.Sprintf("group-%d", i+1),
			AccountID: account.Id,
			Name:      fmt.Sprintf("group-%d", i+1),
			Issued:    types.GroupIssuedAPI,
		}
	}

	err = manager.SaveGroups(context.Background(), account.Id, groupAdminUserID, groups, true)
	assert.NoError(t, err, "Failed to save test groups")

	testCases := []struct {
		name               string
		groupIDs           []string
		expectedReasons    []string
		expectedDeleted    []string
		expectedNotDeleted []string
	}{
		{
			name:            "route",
			groupIDs:        []string{"grp-for-route"},
			expectedReasons: []string{"route"},
		},
		{
			name:            "route with peer groups",
			groupIDs:        []string{"grp-for-route2"},
			expectedReasons: []string{"route"},
		},
		{
			name:            "name server groups",
			groupIDs:        []string{"grp-for-name-server-grp"},
			expectedReasons: []string{"name server groups"},
		},
		{
			name:            "policy",
			groupIDs:        []string{"grp-for-policies"},
			expectedReasons: []string{"policy"},
		},
		{
			name:            "setup keys",
			groupIDs:        []string{"grp-for-keys"},
			expectedReasons: []string{"setup key"},
		},
		{
			name:            "users",
			groupIDs:        []string{"grp-for-users"},
			expectedReasons: []string{"user"},
		},
		{
			name:            "integration",
			groupIDs:        []string{"grp-for-integration"},
			expectedReasons: []string{"only service users with admin power can delete integration group"},
		},
		{
			name:            "successfully delete multiple groups",
			groupIDs:        []string{"group-1", "group-2"},
			expectedDeleted: []string{"group-1", "group-2"},
		},
		{
			name:            "delete non-existent group",
			groupIDs:        []string{"non-existent-group"},
			expectedReasons: []string{"group: non-existent-group not found"},
		},
		{
			name:               "delete multiple groups with mixed results",
			groupIDs:           []string{"group-3", "grp-for-policies", "group-4", "grp-for-users"},
			expectedReasons:    []string{"policy", "user"},
			expectedDeleted:    []string{"group-3", "group-4"},
			expectedNotDeleted: []string{"grp-for-policies", "grp-for-users"},
		},
		{
			name:               "delete groups with multiple errors",
			groupIDs:           []string{"grp-for-policies", "grp-for-users"},
			expectedReasons:    []string{"policy", "user"},
			expectedNotDeleted: []string{"grp-for-policies", "grp-for-users"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = am.DeleteGroups(context.Background(), account.Id, groupAdminUserID, tc.groupIDs)
			if len(tc.expectedReasons) > 0 {
				assert.Error(t, err)
				var foundExpectedErrors int

				wrappedErr, ok := err.(interface{ Unwrap() []error })
				assert.Equal(t, ok, true)

				for _, e := range wrappedErr.Unwrap() {
					var sErr *status.Error
					if errors.As(e, &sErr) {
						assert.Contains(t, tc.expectedReasons, sErr.Message, "unexpected error message")
						foundExpectedErrors++
					}

					var gErr *GroupLinkError
					if errors.As(e, &gErr) {
						assert.Contains(t, tc.expectedReasons, gErr.Resource, "unexpected error resource")
						foundExpectedErrors++
					}
				}
				assert.Equal(t, len(tc.expectedReasons), foundExpectedErrors, "not all expected errors were found")
			} else {
				assert.NoError(t, err)
			}

			for _, groupID := range tc.expectedDeleted {
				_, err := am.GetGroup(context.Background(), account.Id, groupID, groupAdminUserID)
				assert.Error(t, err, "group should have been deleted: %s", groupID)
			}

			for _, groupID := range tc.expectedNotDeleted {
				group, err := am.GetGroup(context.Background(), account.Id, groupID, groupAdminUserID)
				assert.NoError(t, err, "group should not have been deleted: %s", groupID)
				assert.NotNil(t, group, "group should exist: %s", groupID)
			}
		})
	}
}

func initTestGroupAccount(am *DefaultAccountManager) (*DefaultAccountManager, *types.Account, error) {
	accountID := "testingAcc"
	domain := "example.com"

	groupForRoute := &types.Group{
		ID:        "grp-for-route",
		AccountID: "account-id",
		Name:      "Group for route",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForRoute2 := &types.Group{
		ID:        "grp-for-route2",
		AccountID: "account-id",
		Name:      "Group for route",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForNameServerGroups := &types.Group{
		ID:        "grp-for-name-server-grp",
		AccountID: "account-id",
		Name:      "Group for name server groups",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForPolicies := &types.Group{
		ID:        "grp-for-policies",
		AccountID: "account-id",
		Name:      "Group for policies",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForSetupKeys := &types.Group{
		ID:        "grp-for-keys",
		AccountID: "account-id",
		Name:      "Group for setup keys",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForUsers := &types.Group{
		ID:        "grp-for-users",
		AccountID: "account-id",
		Name:      "Group for users",
		Issued:    types.GroupIssuedAPI,
		Peers:     make([]string, 0),
	}

	groupForIntegration := &types.Group{
		ID:        "grp-for-integration",
		AccountID: "account-id",
		Name:      "Group for users integration",
		Issued:    types.GroupIssuedIntegration,
		Peers:     make([]string, 0),
	}

	routeResource := &route.Route{
		ID:     "example route",
		Groups: []string{groupForRoute.ID},
	}

	routePeerGroupResource := &route.Route{
		ID:         "example route with peer groups",
		PeerGroups: []string{groupForRoute2.ID},
	}

	nameServerGroup := &nbdns.NameServerGroup{
		ID:     "example name server group",
		Groups: []string{groupForNameServerGroups.ID},
	}

	policy := &types.Policy{
		ID: "example policy",
		Rules: []*types.PolicyRule{
			{
				ID:           "example policy rule",
				Destinations: []string{groupForPolicies.ID},
			},
		},
	}

	setupKey := &types.SetupKey{
		Id:         "example setup key",
		AutoGroups: []string{groupForSetupKeys.ID},
		UpdatedAt:  time.Now(),
	}

	user := &types.User{
		Id:         "example user",
		AutoGroups: []string{groupForUsers.ID},
	}
	account := newAccountWithId(context.Background(), accountID, groupAdminUserID, domain, false)
	account.Routes[routeResource.ID] = routeResource
	account.Routes[routePeerGroupResource.ID] = routePeerGroupResource
	account.NameServerGroups[nameServerGroup.ID] = nameServerGroup
	account.Policies = append(account.Policies, policy)
	account.SetupKeys[setupKey.Id] = setupKey
	account.Users[user.Id] = user

	err := am.Store.SaveAccount(context.Background(), account)
	if err != nil {
		return nil, nil, err
	}

	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForRoute, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForRoute2, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForNameServerGroups, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForPolicies, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForSetupKeys, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForUsers, true)
	_ = am.SaveGroup(context.Background(), accountID, groupAdminUserID, groupForIntegration, true)

	acc, err := am.Store.GetAccount(context.Background(), account.Id)
	if err != nil {
		return nil, nil, err
	}
	return am, acc, nil
}

func TestGroupAccountPeersUpdate(t *testing.T) {
	manager, account, peer1, peer2, peer3 := setupNetworkMapTest(t)

	err := manager.SaveGroups(context.Background(), account.Id, userID, []*types.Group{
		{
			ID:    "groupA",
			Name:  "GroupA",
			Peers: []string{peer1.ID, peer2.ID},
		},
		{
			ID:    "groupB",
			Name:  "GroupB",
			Peers: []string{},
		},
		{
			ID:    "groupC",
			Name:  "GroupC",
			Peers: []string{peer1.ID, peer3.ID},
		},
		{
			ID:    "groupD",
			Name:  "GroupD",
			Peers: []string{},
		},
		{
			ID:    "groupE",
			Name:  "GroupE",
			Peers: []string{peer2.ID},
		},
	}, true)
	assert.NoError(t, err)

	updMsg := manager.peersUpdateManager.CreateChannel(context.Background(), peer1.ID)
	t.Cleanup(func() {
		manager.peersUpdateManager.CloseChannel(context.Background(), peer1.ID)
	})

	// Saving a group that is not linked to any resource should not update account peers
	t.Run("saving unlinked group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldNotReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupB",
			Name:  "GroupB",
			Peers: []string{peer1.ID, peer2.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldNotReceiveUpdate")
		}
	})

	// Adding a peer to a group that is not linked to any resource should not update account peers
	// and not send peer update
	t.Run("adding peer to unlinked group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldNotReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.GroupAddPeer(context.Background(), account.Id, "groupB", peer3.ID)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldNotReceiveUpdate")
		}
	})

	// Removing a peer from a group that is not linked to any resource should not update account peers
	// and not send peer update
	t.Run("removing peer from unliked group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldNotReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.GroupDeletePeer(context.Background(), account.Id, "groupB", peer3.ID)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldNotReceiveUpdate")
		}
	})

	// Deleting group should not update account peers and not send peer update
	t.Run("deleting group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldNotReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.DeleteGroup(context.Background(), account.Id, userID, "groupB")
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldNotReceiveUpdate")
		}
	})

	// adding a group to policy
	_, err = manager.SavePolicy(context.Background(), account.Id, userID, &types.Policy{
		Enabled: true,
		Rules: []*types.PolicyRule{
			{
				Enabled:       true,
				Sources:       []string{"groupA"},
				Destinations:  []string{"groupA"},
				Bidirectional: true,
				Action:        types.PolicyTrafficActionAccept,
			},
		},
	}, true)
	assert.NoError(t, err)

	// Saving a group linked to policy should update account peers and send peer update
	t.Run("saving linked group to policy", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupA",
			Name:  "GroupA",
			Peers: []string{peer1.ID, peer2.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// adding peer to a used group should update account peers and send peer update
	t.Run("adding peer to linked group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.GroupAddPeer(context.Background(), account.Id, "groupA", peer3.ID)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// removing peer from a linked group should update account peers and send peer update
	t.Run("removing peer from linked group", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.GroupDeletePeer(context.Background(), account.Id, "groupA", peer3.ID)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// Saving a group linked to name server group should update account peers and send peer update
	t.Run("saving group linked to name server group", func(t *testing.T) {
		_, err = manager.CreateNameServerGroup(
			context.Background(), account.Id, "nsGroup", "nsGroup", []nbdns.NameServer{{
				IP:     netip.MustParseAddr("1.1.1.1"),
				NSType: nbdns.UDPNameServerType,
				Port:   nbdns.DefaultDNSPort,
			}},
			[]string{"groupC"},
			true, nil, true, userID, false,
		)
		assert.NoError(t, err)

		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err := manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupC",
			Name:  "GroupC",
			Peers: []string{peer1.ID, peer3.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// Saving a group linked to route should update account peers and send peer update
	t.Run("saving group linked to route", func(t *testing.T) {
		newRoute := route.Route{
			ID:          "route",
			Network:     netip.MustParsePrefix("192.168.0.0/16"),
			NetID:       "superNet",
			NetworkType: route.IPv4Network,
			PeerGroups:  []string{"groupA"},
			Description: "super",
			Masquerade:  false,
			Metric:      9999,
			Enabled:     true,
			Groups:      []string{"groupC"},
		}
		_, err := manager.CreateRoute(
			context.Background(), account.Id, newRoute.Network, newRoute.NetworkType, newRoute.Domains, newRoute.Peer,
			newRoute.PeerGroups, newRoute.Description, newRoute.NetID, newRoute.Masquerade, newRoute.Metric,
			newRoute.Groups, []string{}, true, userID, newRoute.KeepRoute,
		)
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err = manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupA",
			Name:  "GroupA",
			Peers: []string{peer1.ID, peer2.ID, peer3.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// Saving a group linked to dns settings should update account peers and send peer update
	t.Run("saving group linked to dns settings", func(t *testing.T) {
		err := manager.SaveDNSSettings(context.Background(), account.Id, userID, &types.DNSSettings{
			DisabledManagementGroups: []string{"groupD"},
		})
		assert.NoError(t, err)

		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err = manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupD",
			Name:  "GroupD",
			Peers: []string{peer1.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})

	// Saving a group linked to network router should update account peers and send peer update
	t.Run("saving group linked to network router", func(t *testing.T) {
		permissionsManager := permissions.NewManager(manager.Store)
		groupsManager := groups.NewManager(manager.Store, permissionsManager, manager)
		resourcesManager := resources.NewManager(manager.Store, permissionsManager, groupsManager, manager)
		routersManager := routers.NewManager(manager.Store, permissionsManager, manager)
		networksManager := networks.NewManager(manager.Store, permissionsManager, resourcesManager, routersManager, manager)

		network, err := networksManager.CreateNetwork(context.Background(), userID, &networkTypes.Network{
			ID:          "network_test",
			AccountID:   account.Id,
			Name:        "network_test",
			Description: "",
		})
		require.NoError(t, err)

		_, err = routersManager.CreateRouter(context.Background(), userID, &routerTypes.NetworkRouter{
			ID:         "router_test",
			NetworkID:  network.ID,
			AccountID:  account.Id,
			PeerGroups: []string{"groupE"},
			Masquerade: true,
			Metric:     9999,
			Enabled:    true,
		})
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			peerShouldReceiveUpdate(t, updMsg)
			close(done)
		}()

		err = manager.SaveGroup(context.Background(), account.Id, userID, &types.Group{
			ID:    "groupE",
			Name:  "GroupE",
			Peers: []string{peer2.ID, peer3.ID},
		}, true)
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("timeout waiting for peerShouldReceiveUpdate")
		}
	})
}
