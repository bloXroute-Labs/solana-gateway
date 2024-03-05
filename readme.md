[![](images/logo.png)](https://bloxroute.com)
<p align="center">
  <a href="https://github.com/bloXroute-Labs/solana-gateway/blob/main/LICENSE.md">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="bxgateway is released under the MIT license." />
  </a>
  <a href="https://github.com/bloXroute-Labs/solana-gateway/blob/main/CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs welcome!" />
  </a>
  <a href="https://discord.gg/FAYTEMm">
    <img alt="Discord" src="https://img.shields.io/discord/638409433860407300?logo=discord">  
  </a>
  <a href="https://twitter.com/intent/follow?screen_name=bloxroutelabs">
    <img alt="Twitter Follow" src="https://img.shields.io/twitter/follow/bloxroutelabs?style=social">  
  </a>
</p>

![image](https://miro.medium.com/v2/resize:fit:1400/0*VEFSDxpBHk05Omju)

# Solana Gateway

bloXroute Solana Blockchain Distribution Network (BDN) sends and delivers shreds **fast**. Expected latency improvements from using the BDN is 30-50ms. The BDN benefits all nodes including Validators and RPC nodes. Solana Gateway is the entrypoint to Solana BDN.

## Quick start

You can choose either to either run locally or [Docker] (recommended). Refer to our technical [documentation][documentation] for full usage instructions.

## Building the source

Building gateway requires Go (version 1.21 or later). You can install it using your favourite package manager. Once
the dependencies are installed, run:

```
make build
```

## Contributing

Please read our [contributing guide] contributing guide

## Documentation

You can find our full technical documentation and architecture [on our website][documentation].

## Troubleshooting

Contact us at [our Discord] for further questions.

[docker]: https://www.docker.com

[documentation]: https://docs.bloxroute.com/solana/solana-bdn

[our Discord]: https://discord.gg/gUBHG82HKP

[contributing guide]: https://github.com/bloXroute-Labs/solana-gateway/blob/main/CONTRIBUTING.md