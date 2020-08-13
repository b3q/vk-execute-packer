# vk-execute-packer (WIP)

Пакер запросов для либы [vksdk](https://github.com/SevereCloud/vksdk)

```
go get github.com/cqln/vk-execute-packer
```

### Пример
```go
package main

import (
	"fmt"
	"os"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/params"
	packer "github.com/cqln/vk-execute-packer"
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
// батчить только Groups.getMembers и Board.getTopics
packer.Default(vk, packer.Rules(packer.Allow,  "Groups.getMembers", "Board.getTopics"))

// батчить все методы кроме Messages.send и Account.ban
packer.Default(vk, packer.Rules(packer.Ignore,  "Messages.send", "Account.ban"))

// P.S. метод Execute всегда выполняется отдельно
 ```