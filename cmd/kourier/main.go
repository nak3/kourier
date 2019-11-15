package main

import (
	kourierController "kourier/pkg/controller"
	"os"

	// This defines the shared main for injected controllers.
	"knative.dev/pkg/injection/sharedmain"
)

func main() {
	// TODO: do not hardcode this
	_ = os.Setenv("SYSTEM_NAMESPACE", "knative-serving")
	_ = os.Setenv("METRICS_DOMAIN", "knative.dev/samples")

	sharedmain.Main("KourierController",
		kourierController.NewController,
	)
}
