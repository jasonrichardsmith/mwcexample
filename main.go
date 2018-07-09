package main

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	// This is our change to the Pod
	labelPatchExistingLabels = `[{"op":"add","path":"/metadata/labels/thisisanewlabel", "value":"hello"}]`
	labelPatchNoLabels       = `[{"op":"add","path":"/metadata/labels", "value":{"thisisanewlabel":"hello"}}]`

	// This is so we can securely talk to the api
	config = Config{
		CertFile: "/etc/webhook/certs/cert.pem",
		KeyFile:  "/etc/webhook/certs/key.pem",
	}

	// Read https://godoc.org/k8s.io/apimachinery/pkg/runtime#NewScheme
	scheme = runtime.NewScheme()

	// Read https://godoc.org/k8s.io/apimachinery/pkg/runtime/serializer#NewCodecFactory
	codecs = serializer.NewCodecFactory(scheme)
)

func addToScheme(scheme *runtime.Scheme) {
	corev1.AddToScheme(scheme)
	admissionregistrationv1beta1.AddToScheme(scheme)
}

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile string
	KeyFile  string
}

// This is just a helper function for errors
func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

// This is our main logic to mutate
func mutatePods(receivedAdmissionReview v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	log.Info("adding label to pod")

	// The pod definition comes as raw bytes, so we will deserialize into the Pod object.
	raw := receivedAdmissionReview.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		log.Error(err)
		return toAdmissionResponse(err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}

	// We are not admitting pods, just mutating them, so set to true
	reviewResponse.Allowed = true
	// if the key exists
	if val, ok := pod.ObjectMeta.Annotations["mwc-example.jasonrichardsmith.com.exclude"]; ok {
		log.Info("annotation exists")
		// if the key is true we will exclude
		if val == "true" {
			log.Info("excluded due to annotation")
			return &reviewResponse
		}
	}
	if len(pod.ObjectMeta.Labels) > 0 {
		reviewResponse.Patch = []byte(labelPatchExistingLabels)
	} else {
		reviewResponse.Patch = []byte(labelPatchNoLabels)
	}
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	log.Printf("added patch %v", string(reviewResponse.Patch))
	return &reviewResponse
}

// This will just be a wrapper for the http request and response, e2e tests have a
// better level of abstraction
func serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	// This is what we send back it will be nested in a returned AdmissionReview
	var admissionResponse *v1beta1.AdmissionResponse

	// We will attempt to deserialize what was sent into the AdmissionReview
	receivedAdmissionReview := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &receivedAdmissionReview); err != nil {
		log.Error(err)
		admissionResponse = toAdmissionResponse(err)
	} else {
		// Success set with AdmissionResponse returned from our mutator
		admissionResponse = mutatePods(receivedAdmissionReview)
	}
	// This will be the final returned review
	returnedAdmissionReview := v1beta1.AdmissionReview{}

	// We got a return from our mutator
	if admissionResponse != nil {
		// set the response in the returned review
		returnedAdmissionReview.Response = admissionResponse
		// reference the original request
		returnedAdmissionReview.Response.UID = receivedAdmissionReview.Request.UID
	}

	// to json and write to responsewriter
	responseInBytes, err := json.Marshal(returnedAdmissionReview)
	log.Info(string(responseInBytes))

	if err != nil {
		log.Error(err)
		return
	}
	log.Info("Writing response")
	if _, err := w.Write(responseInBytes); err != nil {
		log.Error(err)
	}
}

func init() {

	// intiate the deserializer
	addToScheme(scheme)
}

func main() {
	// Load our cert files
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		log.Fatal(err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}

	// serve it up
	http.HandleFunc("/mutating-pods", serve)
	server := &http.Server{
		Addr:      ":443",
		TLSConfig: tlsConfig,
	}
	server.ListenAndServeTLS("", "")
}
