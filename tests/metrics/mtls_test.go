package metrics_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/monitoring"
)

// mtlsComponents defines the component matrix for mTLS testing.
// Each entry maps a component to its Service(s) and ServiceMonitor(s).
var mtlsComponents = []struct {
	component      string
	services       []monitoring.MTLSComponentConfig
	serviceMonitor string
}{
	{
		component: "TektonPipeline",
		services: []monitoring.MTLSComponentConfig{
			// Prometheus job name = value of 'app' label on the service (via jobLabel: "app" in openshift-pipelines-monitor).
			// tekton-events-controller has no dedicated ServiceMonitor → PrometheusJob left empty → health check skipped.
			{ServiceName: "tekton-pipelines-controller", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "tekton-pipelines-controller"},
			{ServiceName: "tekton-events-controller", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: ""},
			{ServiceName: "tekton-pipelines-remote-resolvers", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "tekton-pipelines-remote-resolvers"},
		},
		serviceMonitor: "openshift-pipelines-monitor",
	},
	{
		component: "TektonTrigger",
		services: []monitoring.MTLSComponentConfig{
			{ServiceName: "tekton-triggers-controller", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "tekton-triggers-controller"},
		},
		serviceMonitor: "openshift-triggers-monitor",
	},
	{
		component: "TektonChain",
		services: []monitoring.MTLSComponentConfig{
			// Service is named "tekton-chains-metrics" but its 'app' label is "tekton-chains-controller",
			// which is what Prometheus uses as the job name (jobLabel: "app" in openshift-chains-monitor).
			{ServiceName: "tekton-chains-metrics", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "tekton-chains-controller"},
		},
		serviceMonitor: "openshift-chains-monitor",
	},
	{
		component: "TektonPruner",
		services: []monitoring.MTLSComponentConfig{
			// PrometheusJob is empty: openshift-pruner-monitor uses jobLabel:"app" but tekton-pruner-controller
			// service may not expose a matching Prometheus target; health check uses the SM assertion instead.
			{ServiceName: "tekton-pruner-controller", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "tekton-pruner-controller"},
		},
		serviceMonitor: "openshift-pruner-monitor",
	},
	{
		component: "OpenShiftPipelinesAsCode",
		services: []monitoring.MTLSComponentConfig{
			{ServiceName: "pipelines-as-code-controller", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "pipelines-as-code-controller"},
			{ServiceName: "pipelines-as-code-watcher", HTTPPortName: "http-metrics", HTTPSPortName: "https-metrics", PrometheusJob: "pipelines-as-code-watcher"},
		},
		serviceMonitor: "pipelines-as-code-controller-monitor",
	},
	{
		component: "TektonResult",
		services: []monitoring.MTLSComponentConfig{
			// tekton-results-watcher uses port name "metrics" (not the standard "http-metrics").
			// The operator renames it to "https-metrics" when mTLS is enabled and back to "metrics" when disabled.
			// PrometheusJob is empty: the SM uses jobLabel:"app" but the service has no "app" label.
			{ServiceName: "tekton-results-watcher", HTTPPortName: "metrics", HTTPSPortName: "https-metrics", PrometheusJob: ""},
		},
		serviceMonitor: "openshift-results-watcher-monitor",
	},
}

// pacExtraServiceMonitor is the additional ServiceMonitor for PAC.
const pacExtraServiceMonitor = "pipelines-as-code-monitor"

// This spec exercises the real operator-managed namespace (config.TargetNamespace) and
// cluster-scoped resources (TektonConfig, TektonInstallerSets). It intentionally opts out of
// per-Describe namespace creation via Label("no-auto-namespace").
var _ = Describe("Prometheus metrics mTLS (enableMetricsMTLS)", Serial, Ordered,
	Label("metrics", "e2e", "admin", "no-auto-namespace"), func() {

		var originalMTLSValue *bool
		var originalPrunerEnabled *bool

		BeforeAll(func() {
			// Save and enable the persistent TektonPruner controller.
			// By default it may be disabled (running as a CronJob only); enabling it deploys
			// the tekton-pruner-controller Service and Deployment that mTLS tests require.
			originalPrunerEnabled = monitoring.GetTektonPrunerEnabled()
			log.Printf("Original tektonpruner enabled: %v", originalPrunerEnabled)
			Expect(monitoring.SetTektonPrunerEnabled(true)).To(Succeed())
			err := monitoring.WaitForTektonConfigReady(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready after enabling TektonPruner")

			// Save the original enableMetricsMTLS value
			originalMTLSValue = monitoring.GetEnableMetricsMTLS()
			log.Printf("Original enableMetricsMTLS value: %v", originalMTLSValue)
		})

		AfterAll(func() {
			// Restore the original enableMetricsMTLS value
			if originalMTLSValue != nil {
				Expect(monitoring.SetEnableMetricsMTLS(*originalMTLSValue)).To(Succeed())
			} else {
				// If it was unset, disable it to restore the default state
				Expect(monitoring.SetEnableMetricsMTLS(false)).To(Succeed())
			}
			log.Printf("Restored enableMetricsMTLS to original value")

			// Restore the original TektonPruner state (re-enables CronJob pruner if it was active before)
			Expect(monitoring.RestoreOriginalPrunerState(originalPrunerEnabled)).To(Succeed())
			log.Printf("Restored TektonPruner enabled state")

			err := monitoring.WaitForTektonConfigReady(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready after cleanup")
		})

		// ── mTLS enabled assertions ─────────────────────────────────────────

		Describe("mTLS enabled", func() {
			BeforeAll(func() {
				Expect(monitoring.SetEnableMetricsMTLS(true)).To(Succeed())
				err := monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready after enabling mTLS")
			})

			for _, comp := range mtlsComponents {
				comp := comp // capture loop variable for closure
				Context(comp.component, func() {
					for _, svc := range comp.services {
						svc := svc // capture loop variable for closure
						svc.Namespace = config.TargetNamespace
						svc.ComponentName = comp.component

						It("Service "+svc.ServiceName+" has mTLS configuration", func() {
							err := monitoring.AssertServiceMTLSEnabled(sharedClients, svc)
							Expect(err).NotTo(HaveOccurred())
						})

						It("Pod for "+svc.ServiceName+" has mTLS env vars and volumes", func() {
							err := monitoring.AssertPodMTLSEnabled(sharedClients, svc)
							Expect(err).NotTo(HaveOccurred())
						})

						It("Prometheus scrape health for "+svc.ServiceName+" reports up=1", func() {
							if svc.PrometheusJob == "" {
								Skip("No Prometheus job configured for " + svc.ServiceName + " (no dedicated ServiceMonitor on this cluster)")
							}
							err := monitoring.VerifyHealthStatusMetric(sharedClients, monitoring.TargetService{
								Job:           svc.PrometheusJob,
								ExpectedValue: "1",
							})
							Expect(err).NotTo(HaveOccurred(),
								"Health status metric check failed for job: %s", svc.PrometheusJob)
						})
					}

					monitorCfg := monitoring.MTLSComponentConfig{
						ServiceMonitorName: comp.serviceMonitor,
						ServiceName:        comp.services[0].ServiceName,
						Namespace:          config.TargetNamespace,
						ComponentName:      comp.component,
					}

					It("ServiceMonitor "+comp.serviceMonitor+" has HTTPS scheme and tlsConfig", func() {
						err := monitoring.AssertServiceMonitorMTLSEnabled(monitorCfg)
						Expect(err).NotTo(HaveOccurred())
					})
				})
			}

			// TLS handshake validation: with-cert succeeds, without-cert is rejected
			It("TLS handshake succeeds with client cert and fails without", func() {
				cfg := monitoring.MTLSComponentConfig{
					ComponentName: "TektonPipeline",
					ServiceName:   "tekton-pipelines-controller",
					Namespace:     config.TargetNamespace,
					HTTPPortName:  "http-metrics",
					HTTPSPortName: "https-metrics",
				}
				err := monitoring.AssertTLSHandshake(sharedClients, cfg)
				Expect(err).NotTo(HaveOccurred())
			})

			// PAC has an extra ServiceMonitor for the watcher (pipelines-as-code-monitor selects app=pipelines-as-code-watcher)
			It("ServiceMonitor "+pacExtraServiceMonitor+" has HTTPS scheme and tlsConfig", func() {
				cfg := monitoring.MTLSComponentConfig{
					ServiceMonitorName: pacExtraServiceMonitor,
					ServiceName:        "pipelines-as-code-watcher",
					Namespace:          config.TargetNamespace,
					ComponentName:      "OpenShiftPipelinesAsCode",
				}
				err := monitoring.AssertServiceMonitorMTLSEnabled(cfg)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		// ── mTLS disabled assertions ────────────────────────────────────────

		Describe("mTLS disabled", func() {
			BeforeAll(func() {
				Expect(monitoring.SetEnableMetricsMTLS(false)).To(Succeed())
				err := monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready after disabling mTLS")
			})

			for _, comp := range mtlsComponents {
				comp := comp // capture loop variable for closure
				Context(comp.component, func() {
					for _, svc := range comp.services {
						svc := svc // capture loop variable for closure
						svc.Namespace = config.TargetNamespace
						svc.ComponentName = comp.component

						It("Service "+svc.ServiceName+" has plain HTTP configuration", func() {
							err := monitoring.AssertServiceMTLSDisabled(sharedClients, svc)
							Expect(err).NotTo(HaveOccurred())
						})

						It("Pod for "+svc.ServiceName+" has no mTLS env vars", func() {
							err := monitoring.AssertPodMTLSDisabled(sharedClients, svc)
							Expect(err).NotTo(HaveOccurred())
						})

						It("Prometheus scrape health for "+svc.ServiceName+" reports up=1 over plain HTTP", func() {
							if svc.PrometheusJob == "" {
								Skip("No Prometheus job configured for " + svc.ServiceName + " (no dedicated ServiceMonitor on this cluster)")
							}
							err := monitoring.VerifyHealthStatusMetric(sharedClients, monitoring.TargetService{
								Job:           svc.PrometheusJob,
								ExpectedValue: "1",
							})
							Expect(err).NotTo(HaveOccurred(),
								"Health status metric check failed for job: %s", svc.PrometheusJob)
						})
					}

					monitorCfg := monitoring.MTLSComponentConfig{
						ServiceMonitorName: comp.serviceMonitor,
						Namespace:          config.TargetNamespace,
						ComponentName:      comp.component,
					}

					It("ServiceMonitor "+comp.serviceMonitor+" has plain HTTP scheme", func() {
						err := monitoring.AssertServiceMonitorMTLSDisabled(monitorCfg)
						Expect(err).NotTo(HaveOccurred())
					})
				})
			}

			It("ServiceMonitor "+pacExtraServiceMonitor+" has plain HTTP scheme", func() {
				cfg := monitoring.MTLSComponentConfig{
					ServiceMonitorName: pacExtraServiceMonitor,
					Namespace:          config.TargetNamespace,
					ComponentName:      "OpenShiftPipelinesAsCode",
				}
				err := monitoring.AssertServiceMonitorMTLSDisabled(cfg)
				Expect(err).NotTo(HaveOccurred())
			})

			// Plain HTTP handshake validation: metrics endpoint is accessible without TLS
			It("plain HTTP handshake succeeds without client cert", func() {
				cfg := monitoring.MTLSComponentConfig{
					ComponentName: "TektonPipeline",
					ServiceName:   "tekton-pipelines-controller",
					Namespace:     config.TargetNamespace,
					HTTPPortName:  "http-metrics",
					HTTPSPortName: "https-metrics",
				}
				err := monitoring.AssertPlainHTTPHandshake(sharedClients, cfg)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		// ── Negative test: tekton-results-api stays plain HTTP always ────────

		Describe("tekton-results-api stays plain HTTP regardless of enableMetricsMTLS", func() {

			// tekton-results-api-service uses port name "prometheus" (not the mTLS-managed "http-metrics").
			// It also carries a pre-existing serving-cert annotation for its main API TLS (unrelated to
			// enableMetricsMTLS). The operator intentionally excludes it from mTLS metrics management.
			// We assert only the ServiceMonitor: openshift-results-api-monitor must never get scheme:https
			// or tlsConfig regardless of the enableMetricsMTLS flag.
			resultsAPISMCfg := monitoring.MTLSComponentConfig{
				ComponentName:      "TektonResult-API",
				ServiceMonitorName: "openshift-results-api-monitor",
				Namespace:          config.TargetNamespace,
			}

			It("stays plain HTTP when mTLS is enabled", func() {
				Expect(monitoring.SetEnableMetricsMTLS(true)).To(Succeed())
				err := monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready")

				By("ServiceMonitor openshift-results-api-monitor stays plain HTTP (not managed by enableMetricsMTLS)")
				err = monitoring.AssertServiceMonitorMTLSDisabled(resultsAPISMCfg)
				Expect(err).NotTo(HaveOccurred())
			})

			It("stays plain HTTP when mTLS is disabled", func() {
				Expect(monitoring.SetEnableMetricsMTLS(false)).To(Succeed())
				err := monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig did not reach Ready")

				By("ServiceMonitor openshift-results-api-monitor stays plain HTTP (not managed by enableMetricsMTLS)")
				err = monitoring.AssertServiceMonitorMTLSDisabled(resultsAPISMCfg)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		// ── Toggle idempotency ──────────────────────────────────────────────

		Describe("toggle idempotency (enable → disable → enable)", func() {

			It("cycles the flag without stuck InstallerSets and TektonConfig stays Ready", func() {
				By("Enable mTLS (first time)")
				Expect(monitoring.SetEnableMetricsMTLS(true)).To(Succeed())
				err := monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig not Ready after enable #1")
				err = monitoring.AssertNoStuckInstallerSets()
				Expect(err).NotTo(HaveOccurred(), "Stuck InstallerSets after enable #1")

				By("Disable mTLS")
				Expect(monitoring.SetEnableMetricsMTLS(false)).To(Succeed())
				err = monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig not Ready after disable")
				err = monitoring.AssertNoStuckInstallerSets()
				Expect(err).NotTo(HaveOccurred(), "Stuck InstallerSets after disable")

				By("Re-enable mTLS (second time)")
				Expect(monitoring.SetEnableMetricsMTLS(true)).To(Succeed())
				err = monitoring.WaitForTektonConfigReady(sharedClients)
				Expect(err).NotTo(HaveOccurred(), "TektonConfig not Ready after enable #2")
				err = monitoring.AssertNoStuckInstallerSets()
				Expect(err).NotTo(HaveOccurred(), "Stuck InstallerSets after enable #2")

				By("Verify mTLS is correctly configured after re-enable")
				// Spot-check the pipelines controller Service
				svcCfg := monitoring.MTLSComponentConfig{
					ServiceName:   "tekton-pipelines-controller",
					Namespace:     config.TargetNamespace,
					HTTPPortName:  "http-metrics",
					HTTPSPortName: "https-metrics",
				}
				err = monitoring.AssertServiceMTLSEnabled(sharedClients, svcCfg)
				Expect(err).NotTo(HaveOccurred(),
					"tekton-pipelines-controller Service should have mTLS after re-enable")
			})
		})
	})
