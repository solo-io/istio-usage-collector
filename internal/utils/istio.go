package utils

// A minified version of istioctl's checkinject command which checks if a pod is automatically injected with an istio sidecar: https://github.com/istio/istio/tree/master/istioctl/pkg/checkinject

import (
	"fmt"
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// FilterIstioWebhooks filters out non-istio webhooks from a list of webhooks
func FilterIstioWebhooks(whs []admissionregistrationv1.MutatingWebhookConfiguration) []admissionregistrationv1.MutatingWebhookConfiguration {
	istioWebhooks := make([]admissionregistrationv1.MutatingWebhookConfiguration, 0)
	for _, mwc := range whs {
		if isIstioWebhook(&mwc) {
			istioWebhooks = append(istioWebhooks, mwc)
		}
	}
	return istioWebhooks
}

// CheckInject checks if a pod is automatically injected with an istio sidecar
// It assumes the passed in mutating webhooks are only istio webhooks
func CheckInject(istioWebhooks []admissionregistrationv1.MutatingWebhookConfiguration, podLabels, nsLabels map[string]string) bool {
	for _, mwc := range istioWebhooks {
		// if any istio webhook is found which injects the container, return true
		if analyzeWebhooksMatchStatus(mwc.Webhooks, podLabels, nsLabels) {
			return true
		}
	}
	return false
}

func analyzeWebhooksMatchStatus(whs []admissionregistrationv1.MutatingWebhook, podLabels, nsLabels map[string]string) (injected bool) {
	for _, wh := range whs {
		nsMatched, nsLabel := extractMatchedSelectorInfo(wh.NamespaceSelector, nsLabels)
		podMatched, podLabel := extractMatchedSelectorInfo(wh.ObjectSelector, podLabels)
		if nsMatched && podMatched {
			if nsLabel != "" && podLabel != "" {
				return true
			} else if nsLabel != "" {
				return true
			} else if podLabel != "" {
				return true
			}
		} else if nsMatched {
			for _, me := range wh.ObjectSelector.MatchExpressions {
				switch me.Operator {
				case metav1.LabelSelectorOpDoesNotExist:
					if _, ok := podLabels[me.Key]; ok {
						return false
					}
				case metav1.LabelSelectorOpNotIn:
					v, ok := podLabels[me.Key]
					if !ok {
						continue
					}
					for _, nv := range me.Values {
						if nv == v {
							return false
						}
					}
				}
			}
		} else if podMatched {
			if v, ok := nsLabels["istio-injection"]; ok {
				if v != "enabled" {
					return false
				}
			}
		}
	}
	return false
}

func extractMatchedSelectorInfo(ls *metav1.LabelSelector, objLabels map[string]string) (matched bool, injLabel string) {
	if ls == nil {
		return true, ""
	}
	selector, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return false, ""
	}
	matched = selector.Matches(labels.Set(objLabels))
	if !matched {
		return matched, ""
	}
	for _, me := range ls.MatchExpressions {
		switch me.Operator {
		case metav1.LabelSelectorOpIn, metav1.LabelSelectorOpNotIn:
			if v, exist := objLabels[me.Key]; exist {
				return matched, fmt.Sprintf("%s=%s", me.Key, v)
			}
		}
	}
	return matched, ""
}

func isIstioWebhook(wh *admissionregistrationv1.MutatingWebhookConfiguration) bool {
	for _, w := range wh.Webhooks {
		if strings.HasSuffix(w.Name, "istio.io") {
			return true
		}
	}
	return false
}
