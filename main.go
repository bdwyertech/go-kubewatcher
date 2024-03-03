//
// Kubewatcher
//
// Copyright © 2024 Brian Dwyer - Intelligent Digital Services. All rights reserved.
//

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/logrusorgru/aurora/v4"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	clientCache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	ctrl.SetLogger(zap.New(zap.ConsoleEncoder()))
	logger := ctrl.Log.WithName("watcher")
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		logger.Error(err, "unable to build K8S RestConfig")
		return
	}
	target := os.Getenv("KUBEWATCHER_RESOURCE")
	if target == "" {
		mapper, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			logger.Error(err, "unable to build K8S discovery client")
			return
		}
		_, list, err := mapper.ServerGroupsAndResources()
		if err != nil {
			// Take a look at `kubectl get apiservices` if you hit an error here
			logger.Error(err, "error discovering APIs")
		}
		if len(list) == 0 {
			logger.Info("no API resources detected")
			return
		}
		var choices []string
		for _, group := range list {
			split := strings.Split(group.GroupVersion, "/")
			var resourceGroup, resourceVersion string
			switch len(split) {
			case 1:
				resourceVersion = split[0]
			case 2:
				resourceGroup = split[0]
				resourceVersion = split[1]
			default:
				logger.Error(nil, "unknown format: "+group.GroupVersion)
			}
			for _, resource := range group.APIResources {
				if strings.Contains(resource.Name, "/") {
					continue
				}
				resource := fmt.Sprintf("%s.%s.%s", resource.Name, resourceVersion, resourceGroup)
				choices = append(choices, resource)
			}
		}
		sort.Strings(choices)
		resourcePrompt := &survey.Select{
			Message: "Choose a resource:",
			Options: choices,
		}
		if err = survey.AskOne(resourcePrompt, &target, survey.WithValidator(survey.Required)); err != nil {
			logger.Error(err, "error choosing resource")
			return
		}
	}

	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		logger.Error(err, "unable to build K8S client")
	}
	f := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, 0, v1.NamespaceAll, nil)
	gvr, _ := schema.ParseResourceArg(target)
	i := f.ForResource(*gvr)
	inf := i.Informer()
	inf.AddEventHandler(clientCache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
		},
		DeleteFunc: func(obj interface{}) {
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			fmt.Println(colorizeDiff(cmp.Diff(oldObj, newObj)))
		},
	})
	ctx := ctrl.SetupSignalHandler()
	inf.Run(ctx.Done())
}

func colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var coloredLines []string

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+"):
			coloredLines = append(coloredLines, aurora.Green(line).String())
		case strings.HasPrefix(line, "-"):
			coloredLines = append(coloredLines, aurora.Red(line).String())
		default:
			coloredLines = append(coloredLines, line)
		}
	}

	return strings.Join(coloredLines, "\n")
}
