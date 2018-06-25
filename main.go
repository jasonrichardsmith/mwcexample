package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile string
	KeyFile  string
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "tls-cert-file", c.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")
	flag.StringVar(&c.KeyFile, "tls-private-key-file", c.KeyFile, ""+
		"File containing the default x509 private key matching --tls-cert-file.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

// only allow pods to pull images from specific registry.
func admitPods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("admitting pods")
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		err := fmt.Errorf("expect resource to be %s", podResource)
		glog.Error(err)
		return toAdmissionResponse(err)
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	var msg string
	if v, ok := pod.Labels["webhook-e2e-test"]; ok {
		if v == "webhook-disallow" {
			reviewResponse.Allowed = false
			msg = msg + "the pod contains unwanted label; "
		}
		if v == "wait-forever" {
			reviewResponse.Allowed = false
			msg = msg + "the pod response should not be sent; "
			<-make(chan int) // Sleep forever - no one sends to this channel
		}
	}
	for _, container := range pod.Spec.Containers {
		if strings.Contains(container.Name, "webhook-disallow") {
			reviewResponse.Allowed = false
			msg = msg + "the pod contains unwanted container name; "
		}
	}
	if !reviewResponse.Allowed {
		reviewResponse.Result = &metav1.Status{Message: strings.TrimSpace(msg)}
	}
	return &reviewResponse
}

func mutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("mutating pods")
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		glog.Errorf("expect resource to be %s", podResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true
	return &reviewResponse
}

// deny configmaps with specific key-value pair.
func admitConfigMaps(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("admitting configmaps")
	configMapResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	if ar.Request.Resource != configMapResource {
		glog.Errorf("expect resource to be %s", configMapResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	configmap := corev1.ConfigMap{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &configmap); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true
	for k, v := range configmap.Data {
		if k == "webhook-e2e-test" && v == "webhook-disallow" {
			reviewResponse.Allowed = false
			reviewResponse.Result = &metav1.Status{
				Reason: "the configmap contains unwanted key and value",
			}
		}
	}
	return &reviewResponse
}

func mutateConfigmaps(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("mutating configmaps")
	configMapResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	if ar.Request.Resource != configMapResource {
		glog.Errorf("expect resource to be %s", configMapResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	configmap := corev1.ConfigMap{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &configmap); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt

	return &reviewResponse
}

func mutateCRD(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("mutating crd")
	cr := struct {
		metav1.ObjectMeta
		Data map[string]string
	}{}

	raw := ar.Request.Object.Raw
	err := json.Unmarshal(raw, &cr)
	if err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return &reviewResponse
}

func admitCRD(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("admitting crd")
	cr := struct {
		metav1.ObjectMeta
		Data map[string]string
	}{}

	raw := ar.Request.Object.Raw
	err := json.Unmarshal(raw, &cr)
	if err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true
	for k, v := range cr.Data {
		if k == "webhook-e2e-test" && v == "webhook-disallow" {
			reviewResponse.Allowed = false
			reviewResponse.Result = &metav1.Status{
				Reason: "the custom resource contains unwanted data",
			}
		}
	}
	return &reviewResponse
}

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Error(err)
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}
	// reset the Object and OldObject, they are not needed in a response.
	ar.Request.Object = runtime.RawExtension{}
	ar.Request.OldObject = runtime.RawExtension{}

	resp, err := json.Marshal(response)
	if err != nil {
		glog.Error(err)
	}
	if _, err := w.Write(resp); err != nil {
		glog.Error(err)
	}
}

func servePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitPods)
}

func serveMutatePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutatePods)
}

func serveConfigmaps(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitConfigMaps)
}

func serveMutateConfigmaps(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutateConfigmaps)
}

func serveCRD(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitCRD)
}

func serveMutateCRD(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutateCRD)
}

// Get a clientset with in-cluster config.
func getClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatal(err)
	}
	return clientset
}

func configTLS(config Config, clientset *kubernetes.Clientset) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		glog.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
		// TODO: uses mutual tls after we agree on what cert the apiserver should use.
		// ClientAuth:   tls.RequireAndVerifyClientCert,
	}
}

func init() {
	addToScheme(scheme)
}

func addToScheme(scheme *runtime.Scheme) {
	corev1.AddToScheme(scheme)
	admissionregistrationv1beta1.AddToScheme(scheme)
}
func main() {
	var config Config
	config.addFlags()
	flag.Parse()

	http.HandleFunc("/pods", servePods)
	http.HandleFunc("/mutating-pods", serveMutatePods)
	http.HandleFunc("/configmaps", serveConfigmaps)
	http.HandleFunc("/mutating-configmaps", serveMutateConfigmaps)
	http.HandleFunc("/crd", serveCRD)
	http.HandleFunc("/mutating-crd", serveMutateCRD)
	clientset := getClient()
	server := &http.Server{
		Addr:      ":443",
		TLSConfig: configTLS(config, clientset),
	}
	server.ListenAndServeTLS("", "")
}
