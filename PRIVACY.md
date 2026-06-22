# Política de Privacidade — Controlinho

**Última atualização:** 21 de junho de 2026
**Desenvolvedor:** Leonardo Soledade Costa
**Contato:** lsoledade@live.com

O Controlinho ("o app") transforma seu celular em um controle (trackpad, teclado,
mídia e energia) para o seu próprio PC com Windows, pela rede local (Wi-Fi/Ethernet)
ou Tailscale.

## Resumo

O Controlinho **não coleta, não armazena e não transmite nenhum dado pessoal** para o
desenvolvedor ou para terceiros. Não há nuvem, contas, login, anúncios ou rastreamento.
Toda a comunicação acontece **diretamente entre o seu celular e o seu PC**, na sua rede.

## Que dados o app processa

- **Comandos de controle** (movimentos do cursor, toques, teclas, texto digitado,
  ações de mídia/energia) trafegam do celular para o PC apenas enquanto você usa o app,
  **dentro da sua rede**, e não são gravados nem enviados a lugar nenhum.
- **Configurações locais** (endereço do PC, PIN de pareamento, sensibilidade etc.)
  ficam **somente no dispositivo** — no seu PC e/ou no armazenamento local do navegador
  do celular.
- O app **não acessa** seus contatos, arquivos, localização, câmera ou microfone.

## Certificado (CA) local

Para oferecer uma conexão HTTPS confiável na rede local, o seu PC gera uma **autoridade
certificadora (CA) local**. A chave privada **nunca sai do seu PC**. Opcionalmente,
**você** escolhe instalar o certificado público dessa CA **no seu próprio celular**,
para que o navegador confie na conexão local. Essa CA serve unicamente para certificar
o seu PC na sua rede; ela não é usada para mais nada, e o desenvolvedor não tem acesso
a ela.

## Pareamento (PIN)

O acesso exige um **PIN/token** gerado e guardado **localmente no seu PC**. Ele nunca é
enviado ao desenvolvedor. Serve para impedir que outros dispositivos na mesma rede
controlem o seu PC.

## Permissões usadas

- **Execução em confiança total (runFullTrust):** necessária para injetar entradas de
  mouse/teclado no Windows (API Win32), incompatível com sandbox.
- **Rede privada / cliente-servidor de internet:** necessária para o PC servir o app e
  aceitar a conexão do celular **na rede local**. O app não abre nenhuma porta para a
  internet.

## Compartilhamento e terceiros

Nenhum. Não há servidores do desenvolvedor; nenhum dado é coletado, compartilhado,
vendido ou enviado a terceiros.

## Crianças

O app não é direcionado a crianças e não coleta dados de ninguém.

## Alterações

Eventuais mudanças nesta política serão publicadas nesta mesma página, com a data de
atualização revisada.

## Contato

Dúvidas sobre esta política: **lsoledade@live.com**
