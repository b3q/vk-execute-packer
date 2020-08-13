# vk-execute-packer (WIP)

Пакер запросов для либы [vksdk](https://github.com/SevereCloud/vksdk)

```
go get github.com/b3q/vk-execute-packer
```

### Пример
```go
package main

import (
	"fmt"
	"os"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/params"
	packer "github.com/b3q/vk-execute-packer"
)

func main() {
	token := os.Getenv("TOKEN")
	vk := api.NewVK(token)
	packer.Default(vk, packer.Debug())

	resp, err := vk.UtilsResolveScreenName(
		params.NewUtilsResolveScreenNameBuilder().
			ScreenName("durov").Params,
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("durov id:", resp.ObjectID)
}
```