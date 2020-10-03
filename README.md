# vk-execute-packer (WIP)
[![Documentation](https://pkg.go.dev/badge/github.com/zweihander/vk-execute-packer/v2)](https://pkg.go.dev/github.com/zweihander/vk-execute-packer/v2?tab=doc)
[![Go Report Card](https://goreportcard.com/badge/github.com/zweihander/vk-execute-packer)](https://goreportcard.com/report/github.com/zweihander/vk-execute-packer)
[![license](https://img.shields.io/github/license/zweihander/vk-execute-packer.svg)](https://github.com/zweihander/vk-execute-packer/blob/master/LICENSE)

Пакер запросов для либы [vksdk](https://github.com/SevereCloud/vksdk)

```
go get github.com/zweihander/vk-execute-packer
```

### Пример
```go
package main

import (
	"fmt"
	"os"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/api/params"
	packer "github.com/zweihander/vk-execute-packer/v2"
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

### Параметры
Параметры передаются в виде аргументов в методы `packer.Default()` и `packer.New()`
 - `packer.Debug()` включает вывод дебаг инфы
 - `packer.Tokens(tokens...)` форсит пакер использовать предоставленные токены для выполнения execute-ов\
 (без этой опции пакер будет использовать токены применяющиеся в запросах)
 - `packer.MaxPackedRequests(num)` устанавливает максимальное кол-во запросов в пачке (максимум 25)
 - `packer.Rules(mode, methods...)` устанавливает правила фильтрации методов\
 Пример:
 ```go
// батчить только messages.send и messages.edit
packer.Default(vk, packer.Rules(packer.Allow, "messages.send", "messages.edit"))

// батчить все методы кроме groups.getMembers и board.getTopics
packer.Default(vk, packer.Rules(packer.Ignore, "groups.getMembers", "board.getTopics"))

// P.S. метод execute всегда выполняется отдельно
 ```