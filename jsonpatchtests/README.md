# Testing your patch first

It is a good idea to test your patch before trying it in your webhook

To do this you can install json-patch

```
$ go install github.com/evanphx/json-patch/cmd/json-patch
```

Write your own patch and apply it to your kubectl output as json. (jq added for pretty printing)

```
$ kubectl get pod -n mwc-test pause -o json | json-patch -p patch1.json | jq .
```
