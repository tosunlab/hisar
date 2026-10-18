package appinfo

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// getModificationTime is the latest change to a file, or to a bundle's executables, Info.plist and signature.
func getModificationTime(path string) (time.Time, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	latest := fileInfo.ModTime()
	if !fileInfo.IsDir() {
		return latest, nil
	}
	contentsPath := filepath.Join(path, "Contents")
	executableDirectory := filepath.Join(contentsPath, "MacOS")
	paths := []string{contentsPath, filepath.Join(contentsPath, "Info.plist"), executableDirectory, filepath.Join(contentsPath, "_CodeSignature", "CodeResources")}
	if entries, err := os.ReadDir(executableDirectory); err == nil {
		for _, entry := range entries {
			paths = append(paths, filepath.Join(executableDirectory, entry.Name()))
		}
	}
	for _, candidatePath := range paths {
		if candidateInfo, err := os.Stat(candidatePath); err == nil && candidateInfo.ModTime().After(latest) {
			latest = candidateInfo.ModTime()
		}
	}
	return latest, nil
}

// DescribeApp reads an app's name, icon and code signature from its bundle on disk.
func (resolver *Resolver) DescribeApp(bundlePath string) App {
	app := App{Path: bundlePath, Name: resolver.getBundleName(bundlePath), Signature: resolver.getSignature(bundlePath)}
	if icon, err := resolver.GetIconPNG(bundlePath, DialogIconSize); err == nil {
		app.IconURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
	}
	return app
}

func (resolver *Resolver) getBundleName(bundlePath string) string {
	resolver.mutex.Lock()
	name, isCached := resolver.namesByPath[bundlePath]
	resolver.mutex.Unlock()
	if isCached {
		return name
	}
	if resolver.system.GetBundleDisplayName != nil {
		name = resolver.system.GetBundleDisplayName(bundlePath)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(bundlePath), ".app")
	}
	resolver.mutex.Lock()
	resolver.namesByPath[bundlePath] = name
	resolver.mutex.Unlock()
	return name
}

// GetIconPNG renders an app's icon at size pixels square, once per bundle path and size.
func (resolver *Resolver) GetIconPNG(bundlePath string, size int) ([]byte, error) {
	key := bundlePath + "\x00" + strconv.Itoa(size)
	resolver.mutex.Lock()
	icon, isCached := resolver.iconsByKey[key]
	resolver.mutex.Unlock()
	if isCached {
		return icon, nil
	}
	if resolver.system.GetBundleIconPNG == nil || bundlePath == "" {
		return nil, errIconUnavailable
	}
	icon, err := resolver.system.GetBundleIconPNG(bundlePath, size)
	if err != nil {
		return nil, err
	}
	resolver.mutex.Lock()
	resolver.iconsByKey[key] = icon
	resolver.mutex.Unlock()
	return icon, nil
}

// getSignature checks a code signature again whenever the bundle or file changed on disk.
func (resolver *Resolver) getSignature(path string) Signature {
	if resolver.system.GetSignature == nil {
		return Signature{}
	}
	return resolver.getCachedSignature(resolver.signaturesByPath, path, resolver.system.GetSignature, resolver.system.GetModificationTime)
}

// GetStrictSignature verifies the bundle's sealed resources as well as its executable before granting an identity.
func (resolver *Resolver) GetStrictSignature(bundlePath string) Signature {
	if resolver.system.GetStrictSignature == nil {
		return Signature{}
	}
	return resolver.getCachedSignature(resolver.strictSignaturesByPath, bundlePath, resolver.system.GetStrictSignature, resolver.system.GetBundleModificationTime)
}

// getTeamID reads the Team ID a file is signed with, without checking the signature.
func (resolver *Resolver) getTeamID(path string) string {
	return resolver.getCachedSignature(resolver.teamIDsByPath, path, func(path string) Signature {
		return Signature{TeamID: resolver.system.GetTeamID(path)}
	}, resolver.system.GetModificationTime).TeamID
}

func (resolver *Resolver) getCachedSignature(cachedByPath map[string]cachedSignature, path string, read func(path string) Signature, getModificationTime func(path string) (time.Time, error)) Signature {
	modificationTime, err := getModificationTime(path)
	if err != nil {
		return read(path)
	}
	resolver.mutex.Lock()
	cached, isCached := cachedByPath[path]
	resolver.mutex.Unlock()
	if isCached && cached.modificationTime.Equal(modificationTime) {
		return cached.signature
	}
	signature := read(path)
	resolver.mutex.Lock()
	cachedByPath[path] = cachedSignature{modificationTime: modificationTime, signature: signature}
	resolver.mutex.Unlock()
	return signature
}
