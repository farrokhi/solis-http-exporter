# solis-http-exporter

Prometheus exporter for the Wi-Fi logging sticks from Solis, including the OEMs such as Autarco.
This program talks to the module over LAN, and does not depend on anything on the cloud, such as 
SolisCloud or the Autarco API.

You can add one or more inverters to the configuration file. This programs scrapes the HTTP interface and converts the raw data into Prometheus metrics.

## The record

A [Solis stick](https://www.solisinverters.com/us/inverter#accessories) answers on HTTP to `GET /inverter.cgi`. It returns a single line with the inverter's status:

```
1802020228090133;780036;202;40.1;900;5.4;34293.5;NO;
```

Fields are: Serial, firmware, model, then temperature in Celsius, power in watts, today's yield and
lifetime yield in kWh, and last the alert state.

Note that the password is your WiFi password, and is set by the inverter itself.

## Configuration

```yaml
inverters:
  - name: house
    address: 192.168.2.164
    username: admin
    password_file: /etc/solis-http-exporter/house.password

  - name: garage
    address: 192.168.2.165
    username: admin
    password_file: /etc/solis-http-exporter/garage.password
    timeout: 10s
```

| Field | Required | Default |
|---|---|---|
| `name` | yes | |
| `address` | yes | |
| `port` | no | `80`, or `443` under `https` |
| `username` | no | `admin` |
| `password` | one of | |
| `password_file` | one of | |
| `timeout` | no | `5s` |
| `scheme` | no | `http` |
| `path` | no | `/inverter.cgi` |


It is recommended to use `password_file` and keep it at mode `0600` next to the config file, instead of the inline `password`.

## How to run

Download the binary or one of the packages (deb, rpm, etc) from the
[releases](https://github.com/farrokhi/solis-http-exporter/releases) page, or build it yourself:

```
go install github.com/farrokhi/solis-http-exporter/cmd/solis_http_exporter@latest
solis_http_exporter --config.file=/etc/solis-http-exporter/config.yml
```

| Flag | Default |
|---|---|
| `--config.file` | `/etc/solis-http-exporter/config.yml` |
| `--web.listen-address` | `:9613` |
| `--web.telemetry-path` | `/metrics` |
| `--log.level` | `info`, or `debug`, `warn`, `error` |

The default `:9613` listens on every address the machine has. Put an IP in front of the port to
narrow it down: `127.0.0.1:9613` for loopback, or `192.168.1.5:9613` for one interface.

`/-/healthy` returns 200 if the process is up and the config is valid. 

## Metrics

The `target` label is to differentiate between multiple inverters, and contains the `name` configured in the config file.

| Metric | Type | Meaning |
|---|---|---|
| `solis_up` | gauge | `1` when the inverter returned a valid record |
| `solis_scrape_duration_seconds` | gauge | How long that one request took |
| `solis_inverter_power_watts` | gauge | Output power right now |
| `solis_inverter_temperature_celsius` | gauge | Inverter temperature |
| `solis_inverter_energy_today_joules` | gauge | Produced since midnight |
| `solis_inverter_energy_joules_total` | counter | Produced over the inverter's life |
| `solis_inverter_alert_active` | gauge | `1` when the alert field says anything but `NO` |
| `solis_inverter_info` | gauge | Always `1`, carrying serial, firmware and model as labels |

Metrics are in Joules instead of kWh, to keep Prometheus happy. That is easy to convert to kWh in Grafana, in case you are building your own dashboards.

You can also import the [sample dashboard](grafana/solis-http-exporter.json) from this repo and
adjust it to your liking.

Note that the today's number reset at midnight. This is why it is a gauge, while
the lifetime number is a counter.

If the answer received from the inverter is not valid (or we get refused connection, timeout, bad credentials, or simply unparsable response), `solis_up` is set to 0. If we get the answer, but a field has incorrect value, e.g. `yield_total` is negative, `solis_up` stays at 1, and other metrics are published as usual.

## Prometheus

```yaml
scrape_configs:
  - job_name: solis
    scrape_interval: 60s
    static_configs:
      - targets: [solis-exporter.example.lan:9613]
```

There is no cache in front of the loggers, this means each scrape is an expensive request for your inverter, so you may not want to reduce the scrape interval.

## Important notes

Solis documents indicate the web password as `123456789`, which is not accurate. That is probably true for an "unconfigured" solis hardware. The moment it is configured to joing a Wi-Fi network, the password changes to the Wi-Fi password. 

It is also important to know that the stick is powered by the inverter, and by that it means, it is solar powered. So it is normal for it to "disappear" when it gets dark, and you will be getting no reading from it when there is not enough solar power to power the inverter. Therefore you may not want to use the `solis_up` value for alerting.

## What is next?

You may simply use this code, or you may want to contribute to it. This is an open source project, created to solve my own problem, and it is by no mean complete or perfect. Your contributions are always welcome, in all forms, including bug reports, feature requests, and code contributions.

LLM contributions are welcome as long as you understand the code and can explain what you are doing, and you are not just copy-pasting.

```
go test ./...
go run ./cmd/solis_http_exporter --config.file=examples/config.yml
```

## License

MIT.
