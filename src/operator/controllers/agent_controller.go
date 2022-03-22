package controllers

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	app2scalev1alpha1 "github.com/app2scale/scop/api/v1alpha1"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	//"github.com/prometheus/client_golang/prometheus"
	//"github.com/prometheus/client_golang/prometheus/promauto"
	//"github.com/prometheus/client_golang/prometheus/promhttp"
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// https://github.com/kubecost/cost-model/blob/master/configs/default.json#L5
var cost_cpu float64 = 0.031611 // 1 CPU per hour
var cost_ram float64 = 0.004237 // 1 GB per hour
var rl_logfile, _ = os.Create("data.log")
var epoch int64 = 0
var goodness float64 = 0.0
var oldgoodness float64 = 0.0

var actionDoNothing int64 = 0
var actionIncreaseReplica int64 = 1
var actionDecreaseReplica int64 = 2
var actionIncreaseCPU int64 = 3
var actionDecreaseCPU int64 = 4
var actionIncreaseHeap int64 = 5
var actionDecreaseHeap int64 = 6
var lastAction int64 = 0
var lastReplica int32 = 0
var lastCPU int64 = 0
var lastHeap int = 0

var actionReward = make(map[string]float64)
var learningRate float64 = 0.1

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("<<<<<<<<<<<<<<<<<<<<<<<< reconcile started >>>>>>>>>>>>>>>>>>>>>>>")

	// Target is deployment/teastore-webui
	targetdeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Namespace: "teastore", Name: "teastore-webui"}, targetdeployment)
	if err != nil {
		l.Error(err, "unable to get target deployment")
	}

	// specifications in the deployment
	// Horizontal scaling: spec_replicas
	spec_replicas := *(targetdeployment.Spec.Replicas)
	// vertical scaling: CPU
	cpulimit := targetdeployment.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU]
	spec_cpu := cpulimit.MilliValue()
	// app tuning: heap size
	catalina := targetdeployment.Spec.Template.Spec.Containers[0].Env[3]
	reg, _ := regexp.Compile("-Xmx([0-9]+)M")

	spec_heap_str := reg.FindStringSubmatch(catalina.Value)[1]
	spec_heap, _ := strconv.Atoi(spec_heap_str)
	l.Info("specs", "replicas", spec_replicas)
	l.Info("specs", "cpu", spec_cpu)
	l.Info("specs", "catalina", catalina.Value)
	l.Info("specs", "spec_heap", spec_heap)

	// Get resource metrics
	restconfig := ctrl.GetConfigOrDie()
	tlsClientConfig := rest.TLSClientConfig{}
	tlsClientConfig.Insecure = true
	restconfig.TLSClientConfig = tlsClientConfig
	clientset, _ := metricsv.NewForConfig(restconfig)
	podMetricsList, err := clientset.MetricsV1beta1().PodMetricses("teastore").List(ctx, metav1.ListOptions{LabelSelector: "run=teastore-webui"})

	if err != nil {
		l.Error(err, "Unable to get pod resource metrics")
	}
	var cpu_sum = 0.0
	var ram_sum = 0.0
	for _, podMetrics := range podMetricsList.Items {
		usage_memory := float64(podMetrics.Containers[0].Usage.Memory().MilliValue()) / 1000 / 1024 / 1024 / 1024 // GB
		usage_cpu := float64(podMetrics.Containers[0].Usage.Cpu().MilliValue()) / 1000                            // cpu
		l.Info("podmetrics", "Name", podMetrics.ObjectMeta.Name,
			"CPU", usage_cpu,
			"Memory", usage_memory,
		)
		cpu_sum += usage_cpu
		ram_sum += usage_memory
	}
	cpu_avg := cpu_sum / float64(spec_replicas)
	ram_avg := ram_sum / float64(spec_replicas)
	var cost = (ram_avg*cost_ram + cpu_avg*cost_cpu) * float64(spec_replicas)

	l.Info("Reward", "Cost dollar/hour", cost)

	tlsconfig := &tls.Config{InsecureSkipVerify: true}
	roundtripper := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsconfig,
	}

	authroundtripper := promconfig.NewAuthorizationCredentialsRoundTripper("Bearer", promconfig.Secret(restconfig.BearerToken), roundtripper)
	promcfg := promapi.Config{Address: "https://grafana-openshift-monitoring.apps.scop.tr.asseco-see.local/api/datasources/proxy/1/", RoundTripper: authroundtripper}

	promclient, err := promapi.NewClient(promcfg)
	if err != nil {
		l.Error(err, "promclient", promclient)
	}

	promv1api := promv1.NewAPI(promclient)

	promctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	// Get pods running under teastore-webui
	podlist := &v1.PodList{}
	opts := client.MatchingLabels{"run": "teastore-webui"}
	r.List(ctx, podlist, opts)
	for _, pod := range podlist.Items {
		l.Info("pod", "Name", pod.Name, "Phase", pod.Status.Phase)
	}
	var incomingtps_sum = 0.0
	var incomingtps_count = 0
	var outgoingtps_sum = 0.0
	var outgoingtps_count = 0
	var max_session_rate = 0
	for _, pod := range podlist.Items {
		l.Info("pod", "Name", pod.Name, "Phase", pod.Status.Phase, "Status", pod.Status)
		result, warnings, err := promv1api.Query(promctx, "rate(container_network_receive_packets_total{pod=\""+pod.Name+"\"}[1m])", time.Now())
		if err != nil {
			l.Error(err, "Error querying Prometheus:", "result", result, "warnings", warnings)
		}
		if len(warnings) > 0 {
			l.Info("prometheus", "Warnings", warnings)
		}
		resultVector := result.(model.Vector)
		if resultVector != nil && len(resultVector) > 0 {
			l.Info("Prometheus", "resultVector", resultVector)
			l.Info("Prometheus", "Pod", pod.Name, "Incoming net", resultVector[0].Value)
			if incomingtps, err := strconv.ParseFloat(resultVector[0].Value.String(), 32); err == nil {
				incomingtps_sum += incomingtps
				incomingtps_count++
			}
		}

		result, warnings, err = promv1api.Query(promctx, "rate(container_network_transmit_packets_total{pod=\""+pod.Name+"\"}[1m])", time.Now())
		if err != nil {
			l.Error(err, "Error querying Prometheus:", "result", result, "warnings", warnings)
		}
		if len(warnings) > 0 {
			l.Info("prometheus", "Warnings", warnings)
		}
		resultVector = result.(model.Vector)
		if resultVector != nil && len(resultVector) > 0 {
			l.Info("Prometheus", "Pod", pod.Name, "Outgoing net", resultVector[0].Value)
			if outgoingtps, err := strconv.ParseFloat(resultVector[0].Value.String(), 32); err == nil {
				outgoingtps_sum += outgoingtps
				outgoingtps_count++
			}
		}

	}
	inc_tps := incomingtps_sum / float64(incomingtps_count)
	out_tps := outgoingtps_sum / float64(outgoingtps_count)
	l.Info("Reward", "Mean Incoming TPS", inc_tps)
	l.Info("Reward", "Mean Outgoing TPS", out_tps)

	result, warnings, err := promv1api.Query(promctx, "sum by(exported_pod) (haproxy_server_max_session_rate{exported_service=\"teastore-webui\"})", time.Now())
	if err != nil {
		l.Error(err, "Error querying Prometheus:", "result", result, "warnings", warnings)
	}
	if len(warnings) > 0 {
		l.Info("prometheus", "Warnings", warnings)
	}
	resultVector := result.(model.Vector)
	l.Info("Prometheus", "max session rates", resultVector)
	max_session_rate = 0
	if resultVector != nil && len(resultVector) > 0 {
		for _, res := range resultVector {
			l.Info("Prometheus", "max session rate", res.Value)
			max_session_rate_per_pod, err := strconv.Atoi(res.Value.String())
			if err != nil {
				l.Error(err, "Parse Error", "Atoi", res.Value.String())
			}
			if max_session_rate_per_pod > max_session_rate {
				max_session_rate = max_session_rate_per_pod
			}
		}
	}

	result, warnings, err = promv1api.Query(promctx, "sum by(pod) (haproxy_backend_current_session_rate{route=\"teastore-webui\"})", time.Now())
	if err != nil {
		l.Error(err, "Error querying Prometheus:", "result", result, "warnings", warnings)
	}
	if len(warnings) > 0 {
		l.Info("prometheus", "Warnings", warnings)
	}
	resultVector = result.(model.Vector)
	l.Info("Prometheus", "curr session rates", resultVector)
	cur_session_rate := 0
	if resultVector != nil && len(resultVector) > 0 {
		for _, res := range resultVector {
			l.Info("Prometheus", "cur session rate", res.Value)
			cur_session_rate_per_pod, err := strconv.Atoi(res.Value.String())
			if err != nil {
				l.Error(err, "Parse Error", "Atoi", res.Value.String())
			}
			cur_session_rate += cur_session_rate_per_pod
		}
	}

	perf := float64(cur_session_rate)
	goodness = perf - cost*10000.0
	l.Info("Reward", "Performance", perf)
	l.Info("Reward", "Cost*10000", cost*10000.0)
	l.Info("Reward", "Goodness", goodness)

	if epoch%10 == 0 {
		rl_logfile.WriteString(fmt.Sprintf("%2s %4s %4s %8s %8s %8s %8s %8s %8s %8s %9s %9s %2s %4s %5s %15s %8s\n",
			"re", "heap", "cpu", "inc_tps", "out_tps", "max_sess", "curr_sess", "cpu", "ram", "cost", "goodness", "reward", "re", "heap", "cpu",
			"actionstate", "Qvalue"))
	}
	// Only after the first iteration, I have oldgoodness value
	if epoch > 0 {
		reward := goodness - oldgoodness
		stateActionHash := fmt.Sprintf("%d-%d-%d-%d", lastReplica, lastHeap, lastCPU, lastAction)
		actionReward[stateActionHash] += learningRate * (reward - actionReward[stateActionHash])

		// Actions
		// uniform spacing is crucial
		//range_cpu = [3]int64{100, 500, 900}
		//range_heap = [3]int64{300, 500, 700}
		//range_replicas = [3]int64{1, 2, 3}
		//
		var selectedAction int64
		for {
			selectedAction = rand.Int63n(7)
			if selectedAction == actionDecreaseCPU && spec_cpu == 100 {
				continue
			} else if selectedAction == actionIncreaseCPU && spec_cpu == 900 {
				continue
			} else if selectedAction == actionDecreaseReplica && spec_replicas == 1 {
				continue
			} else if selectedAction == actionIncreaseReplica && spec_replicas == 9 {
				continue
			} else if selectedAction == actionDecreaseHeap && spec_heap == 100 {
				continue
			} else if selectedAction == actionIncreaseHeap && spec_heap == 900 {
				continue
			} else {
				break
			}
		}
		l.Info("Reward", "Selected Action", selectedAction)

		cpu_new_int := spec_cpu
		replica_new := spec_replicas
		heap_new_int := spec_heap
		switch selectedAction {
		case actionDecreaseCPU:
			cpu_new_int -= 100
		case actionIncreaseCPU:
			cpu_new_int += 100
		case actionDecreaseHeap:
			heap_new_int -= 100
		case actionIncreaseHeap:
			heap_new_int += 100
		case actionIncreaseReplica:
			replica_new += 1
		case actionDecreaseReplica:
			replica_new -= 1
		}

		cpu_new := resource.NewMilliQuantity(cpu_new_int, resource.DecimalSI)
		targetdeployment.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = *cpu_new
		targetdeployment.Spec.Replicas = &replica_new
		targetdeployment.Spec.Template.Spec.Containers[0].Env[3].Value = fmt.Sprintf("-Xmx%dM", heap_new_int)

		err = r.Update(ctx, targetdeployment)
		if err != nil {
			l.Error(err, "Failed to update CPU resource", "targetdeployment", targetdeployment)
			return ctrl.Result{}, err
		}

		if epoch%10 == 0 {
			rl_logfile.WriteString(fmt.Sprintf("%2s %4s %4s %8s %8s %8s %8s %8s %8s %8s %9s %9s %2s %4s %5s %15s %8s\n",
				"re", "heap", "cpu", "inc_tps", "out_tps", "max_sess", "cur_sess", "cpu", "ram", "cost", "goodness", "reward", "re", "heap", "cpu",
				"actionstate", "Qvalue"))
		}
		rl_logfile.WriteString(fmt.Sprintf("%2d %4d %4d %8.3f %8.3f %8d %8d %8.4f %8.4f %8.4f %9.4f %9.4f %2d %4d %5.2f %15s %8.4f\n",
			spec_replicas, spec_heap, spec_cpu, inc_tps, out_tps, max_session_rate, cur_session_rate, cpu_avg, ram_avg, cost, goodness, reward, replica_new, heap_new_int, float64(cpu_new_int)/1000.0,
			stateActionHash, actionReward[stateActionHash]))
		lastAction = selectedAction

	}

	if epoch == 0 {
		rl_logfile.WriteString(fmt.Sprintf("%2d %4d %4d %8.3f %8.3f %8d %8d %8.4f %8.4f %8.4f %9.4f %9.4f %2d %4d %5.2f %15s %8.4f\n",
			spec_replicas, spec_heap, spec_cpu, inc_tps, out_tps, max_session_rate, cur_session_rate, cpu_avg, ram_avg, cost, goodness, -1.0, -1, -1, -1.0, "-1", -1.0))
	}

	oldgoodness = goodness
	lastReplica = spec_replicas
	lastCPU = spec_cpu
	lastHeap = spec_heap
	epoch++
	return ctrl.Result{}, nil
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&app2scalev1alpha1.Agent{}).
		Complete(r)
}
