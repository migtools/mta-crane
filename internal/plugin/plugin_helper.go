package plugin

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/konveyor/crane-lib/transform"
	binary_plugin "github.com/konveyor/crane-lib/transform/binary-plugin"
	"github.com/konveyor/crane-lib/transform/kubernetes"
	"github.com/migtools/crane-plugin-buildconfig-to-shipwright/buildconfig"
	"github.com/migtools/crane-plugin-openshift/openshift"
	"github.com/sirupsen/logrus"
)

const (
	DefaultLocalPluginDir = "/.local/share/crane/plugins"
	GlobalPluginDir       = "/usr/local/share/crane/plugins"
	PkgPluginDir          = "/usr/share/crane/plugins"
)

type builtInPlugin struct {
	plugin           transform.Plugin
	enabledByDefault bool
}

func getBuiltInPlugins(logger *logrus.Logger) []builtInPlugin {
	return []builtInPlugin{
		{plugin: &kubernetes.KubernetesTransformPlugin{}, enabledByDefault: true},
		{plugin: &openshift.OpenShiftTransformPlugin{Log: logger}, enabledByDefault: true},
		{plugin: &buildconfig.BuildConfigTransformPlugin{Log: logger}, enabledByDefault: false},
	}
}

func GetPlugins(dir string, logger *logrus.Logger) ([]transform.Plugin, error) {
	pluginList := []transform.Plugin{}
	files, err := ioutil.ReadDir(dir)
	switch {
	case os.IsNotExist(err):
		return pluginList, nil
	case err != nil:
		return nil, err
	}
	list, err := getBinaryPlugins(dir, files, logger)
	if err != nil {
		return nil, err
	}
	pluginList = append(pluginList, list...)
	return pluginList, nil
}

func getBinaryPlugins(path string, files []os.FileInfo, logger *logrus.Logger) ([]transform.Plugin, error) {
	pluginList := []transform.Plugin{}
	for _, file := range files {
		filePath := fmt.Sprintf("%v/%v", path, file.Name())
		if file.IsDir() {
			newFiles, err := ioutil.ReadDir(filePath)
			if err != nil {
				return nil, err
			}
			plugins, err := getBinaryPlugins(filePath, newFiles, logger)
			if err != nil {
				return nil, err
			}
			pluginList = append(pluginList, plugins...)
		} else if file.Mode().IsRegular() && IsExecAny(file.Mode().Perm()) {
			newPlugin, err := binary_plugin.NewBinaryPlugin(filePath, logger)
			if err != nil {
				return nil, err
			}
			pluginList = append(pluginList, newPlugin)
		}
	}
	return pluginList, nil
}

func IsExecAny(mode os.FileMode) bool {
	return mode&0111 != 0
}

func GetFilteredPlugins(pluginDir string, skipPlugins []string, logger *logrus.Logger) ([]transform.Plugin, error) {
	return getPlugins(pluginDir, skipPlugins, logger, true)
}

// GetDefaultPlugins returns plugins that run when no stage is explicitly requested.
func GetDefaultPlugins(pluginDir string, skipPlugins []string, logger *logrus.Logger) ([]transform.Plugin, error) {
	return getPlugins(pluginDir, skipPlugins, logger, false)
}

// GetOptInPluginNames returns built-in plugins which require an explicit stage.
func GetOptInPluginNames(logger *logrus.Logger) map[string]struct{} {
	names := make(map[string]struct{})
	for _, builtIn := range getBuiltInPlugins(logger) {
		if !builtIn.enabledByDefault {
			names[builtIn.plugin.Metadata().Name] = struct{}{}
		}
	}
	return names
}

func getPlugins(pluginDir string, skipPlugins []string, logger *logrus.Logger, includeOptIn bool) ([]transform.Plugin, error) {
	var filteredPlugins, unfilteredPlugins []transform.Plugin
	absPathPluginDir, err := filepath.Abs("plugins")
	if err != nil {
		return filteredPlugins, err
	}

	for _, builtIn := range getBuiltInPlugins(logger) {
		if includeOptIn || builtIn.enabledByDefault {
			unfilteredPlugins = append(unfilteredPlugins, builtIn.plugin)
		}
	}

	paths := []string{absPathPluginDir, pluginDir, GlobalPluginDir, PkgPluginDir}

	for _, path := range paths {
		plugins, err := GetPlugins(path, logger)
		if err != nil {
			return filteredPlugins, err
		}
		for _, newPlugin := range plugins {
			exists := false
			for _, plugin := range unfilteredPlugins {
				if plugin.Metadata().Name == newPlugin.Metadata().Name {
					exists = true
					break
				}
			}
			if !exists {
				unfilteredPlugins = append(unfilteredPlugins, newPlugin)
			}
		}
	}

	if len(skipPlugins) == 0 {
		return unfilteredPlugins, nil
	}
	for _, thisPlugin := range unfilteredPlugins {
		if !isPluginInList(thisPlugin, skipPlugins) {
			filteredPlugins = append(filteredPlugins, thisPlugin)
		}
	}
	return filteredPlugins, nil
}

func isPluginInList(plugin transform.Plugin, list []string) bool {
	pluginName := plugin.Metadata().Name
	for _, listItem := range list {
		if pluginName == listItem {
			return true
		}
	}
	return false
}
