package plugin

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultPluginsExcludesOptInPlugins(t *testing.T) {
	plugins, err := GetDefaultPlugins(t.TempDir(), nil, logrus.New())
	assert.NoError(t, err)

	names := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		names = append(names, plugin.Metadata().Name)
	}

	assert.Contains(t, names, "KubernetesPlugin")
	assert.Contains(t, names, "OpenShiftPlugin")
	assert.NotContains(t, names, "BuildConfigPlugin")
}
