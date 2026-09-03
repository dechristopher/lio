# gofiber/schema

Package gofiber/schema converts structs to and from form values.

Per-commit benchmark charts: <https://gofiber.github.io/schema/benchmarks/>

## Example

Here's a quick example: we parse POST form values and then decode them into a struct:

```go
// Set a Decoder instance as a package global, because it caches
// meta-data about structs, and an instance can be shared safely.
var decoder = schema.NewDecoder()

type Person struct {
    Name  string
    Phone string
}

func MyHandler(w http.ResponseWriter, r *http.Request) {
    err := r.ParseForm()
    if err != nil {
        // Handle error
    }

    var person Person

    // r.PostForm is a map of our POST form values
    err = decoder.Decode(&person, r.PostForm)
    if err != nil {
        // Handle error
    }

    // Do something with person.Name or person.Phone
}
```

Conversely, contents of a struct can be encoded into form values. Here's a variant of the previous example using the Encoder:

```go
var encoder = schema.NewEncoder()

func MyHttpRequest() {
    person := Person{"Jane Doe", "555-5555"}
    form := url.Values{}

    err := encoder.Encode(person, form)

    if err != nil {
        // Handle error
    }

    // Use form values, for example, with an http client
    client := new(http.Client)
    res, err := client.PostForm("http://my-api.test", form)
}

```

To define custom names for fields, use a struct tag "schema". To not populate certain fields, use a dash for the name and it will be ignored:

```go
type Person struct {
    Name  string `schema:"name,required"`  // custom name, must be supplied
    Phone string `schema:"phone"`          // custom name
    Admin bool   `schema:"-"`              // this field is never set
}
```

The supported field types in the struct are:

* bool
* float variants (float32, float64)
* int variants (int, int8, int16, int32, int64)
* string
* uint variants (uint, uint8, uint16, uint32, uint64)
* struct
* a pointer to one of the above types
* a slice or a pointer to a slice of one of the above types

Unsupported types are simply ignored, however custom types can be registered to be converted.

## Setting Defaults

It is possible to set default values when encoding/decoding by using the `default` tag option. The value of `default` is applied when a field has a zero value, a pointer has a nil value, or a slice is empty.

```go
type Person struct {
    Phone string `schema:"phone,default:+123456"`          // custom name
    Age int     `schema:"age,default:21"`
	Admin bool    `schema:"admin,default:false"`
	Balance float64 `schema:"balance,default:10.0"`
    Friends []string `schema:friends,default:john|bob`
}
```

The `default` tag option is supported for the following types:

* bool
* float variants (float32, float64)
* int variants (int, int8, int16, int32, int64)
* uint variants (uint, uint8, uint16, uint32, uint64)
* string
* a slice of the above types. As shown in the example above, `|` should be used to separate between slice items. 
* a pointer to one of the above types (pointer to slice and slice of pointers are not supported).

> [!NOTE]  
> Because primitive types like int, float, bool, unint and their variants have their default (or zero) values set by Golang, it is not possible to distinguish them from a provided value when decoding/encoding form values. In this case, the value provided by the `default` option tag will be always applied. For example, let's assume that the value submitted in the form for `balance` is `0.0` then the default of `10.0` will be applied, even if `0.0` is part of the form data for the `balance` field. In such cases, it is highly recommended to use pointers to allow schema to distinguish between when a form field has no provided value and when a form has a value equal to the corresponding default set by Golang for a particular type. If the type of the `Balance` field above is changed to `*float64`, then the zero value would be `nil`. In this case, if the form data value for `balance` is `0.0`, then the default will not be applied.

<!-- skip-docs -->
## ☕ Supporters

Fiber is an open-source project that runs on donations to pay the bills, e.g., our domain name, hosting, and serverless infrastructure. If you want to support Fiber, please become a [GitHub Sponsor](https://github.com/sponsors/gofiber).

<p align="center">
  <a href="https://www.coderabbit.ai/?utm_source=gofiber&utm_medium=sponsor&utm_content=readme">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://www.coderabbit.ai/images/logo-dark.svg">
      <img width="280" height="52" alt="CodeRabbit" src="https://www.coderabbit.ai/images/logo-orange.svg">
    </picture>
  </a>
</p>
<p align="center">
  <a href="https://blacksmith.sh/?utm_source=gofiber&utm_medium=sponsor&utm_content=readme">
    <img width="280" height="96" alt="Blacksmith" src="https://raw.githubusercontent.com/gofiber/.github/main/assets/sponsors/blacksmith.png">
  </a>
</p>
<p align="center">
  <sub><b>Tool Sponsors</b> - supporting Fiber with free IDE licenses and AI credits</sub>
</p>
<p align="center">
  <a href="https://www.jetbrains.com/?from=gofiber" title="JetBrains - IDE licenses"><img width="36" height="36" alt="JetBrains" src="https://github.com/JetBrains.png?size=72"></a>
  &nbsp;
  <a href="https://openai.com/?utm_source=gofiber&utm_medium=sponsor&utm_content=readme" title="OpenAI - AI credits"><img width="36" height="36" alt="OpenAI" src="https://github.com/openai.png?size=72"></a>
  &nbsp;
  <a href="https://www.anthropic.com/?utm_source=gofiber&utm_medium=sponsor&utm_content=readme" title="Anthropic - AI credits"><img width="36" height="36" alt="Anthropic" src="https://github.com/anthropics.png?size=72"></a>
</p>

<!-- sponsors -->

### 📅 Monthly Sponsors

<table>
<tr><td valign="top"><strong>🔥 Fiber Guardian</strong></td><td><a href="https://www.coderabbit.ai/?utm_source=cr_org&amp;utm_medium=github" title="@coderabbitai"><img src="https://github.com/coderabbitai.png" width="50" alt="@coderabbitai" /></a></td></tr>
<tr><td valign="top"><strong>☕ Fiber Supporter</strong></td><td><a href="https://ndole.studio" title="@NdoleStudio"><img src="https://github.com/NdoleStudio.png" width="34" alt="@NdoleStudio" /></a></td></tr>
<tr><td valign="top"><strong>🪴 Fiber Friend</strong></td><td><a href="https://github.com/simonheisstpeter" title="@simonheisstpeter"><img src="https://github.com/simonheisstpeter.png" width="32" alt="@simonheisstpeter" /></a></td></tr>
</table>

### 🎁 One-time Sponsors

<table>
<tr><td valign="top"><strong>🚀 Fiber Hero</strong></td><td><a href="https://www.thanks.dev" title="@thnxdev"><img src="https://github.com/thnxdev.png" width="40" alt="@thnxdev" /></a></td></tr>
<tr><td valign="top"><strong>🪴 Fiber Friend</strong></td><td><a href="https://github.com/Gl1tchedPixzl" title="@Gl1tchedPixzl"><img src="https://github.com/Gl1tchedPixzl.png" width="26" alt="@Gl1tchedPixzl" /></a></td></tr>
</table>
<!-- sponsors -->
<!-- skip-docs -->

## License

BSD licensed. See the LICENSE file for details.
