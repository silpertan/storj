// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package audit_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"storj.io/storj/internal/memory"
	"storj.io/storj/internal/testcontext"
	"storj.io/storj/internal/testplanet"
	"storj.io/storj/pkg/audit"
	"storj.io/storj/uplink"
)

func TestVerifierHappyPath(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 6, UplinkCount: 1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		err := planet.Satellites[0].Audit.Service.Close()
		require.NoError(t, err)

		ul := planet.Uplinks[0]
		testData := make([]byte, 1*memory.MiB)
		_, err = rand.Read(testData)
		require.NoError(t, err)

		err = ul.UploadWithConfig(ctx, planet.Satellites[0], &uplink.RSConfig{
			MinThreshold:     4,
			RepairThreshold:  5,
			SuccessThreshold: 6,
			MaxThreshold:     6,
		}, "testbucket", "test/path", testData)
		require.NoError(t, err)

		metainfo := planet.Satellites[0].Metainfo.Service
		overlay := planet.Satellites[0].Overlay.Service
		cursor := audit.NewCursor(metainfo)

		stripe, _, err := cursor.NextStripe(ctx)
		require.NoError(t, err)
		require.NotNil(t, stripe)

		transport := planet.Satellites[0].Transport
		orders := planet.Satellites[0].Orders.Service
		containment := planet.Satellites[0].DB.Containment()
		minBytesPerSecond := 128 * memory.B

		reporter := audit.NewReporter(overlay, containment, 1)
		verifier := audit.NewVerifier(zap.L(), reporter, transport, overlay, containment, orders, planet.Satellites[0].Identity, minBytesPerSecond)
		require.NotNil(t, verifier)

		// stop some storage nodes to ensure audit can deal with it
		err = planet.StopPeer(planet.StorageNodes[0])
		require.NoError(t, err)
		err = planet.StopPeer(planet.StorageNodes[1])
		require.NoError(t, err)

		// mark stopped nodes as offline in overlay cache
		_, err = planet.Satellites[0].Overlay.Service.UpdateUptime(ctx, planet.StorageNodes[0].ID(), false)
		require.NoError(t, err)
		_, err = planet.Satellites[0].Overlay.Service.UpdateUptime(ctx, planet.StorageNodes[1].ID(), false)
		require.NoError(t, err)

		verifiedNodes, err := verifier.Verify(ctx, stripe, nil)
		require.NoError(t, err)

		require.Len(t, verifiedNodes.Successes, 4)
		require.Len(t, verifiedNodes.Fails, 0)
	})
}
