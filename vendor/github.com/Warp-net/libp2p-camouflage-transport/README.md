# libp2p-camouflage-transport

 Package provides a libp2p transport wrapper that defeats Deep Packet
 Inspection through two complementary techniques:

  1. TLS camouflage – a real TLS tunnel is established using uTLS with a
     genuine browser fingerprint (Chrome, Firefox, etc.) on the client side
     and standard crypto/tls with a plausible certificate chain on the server
     side. The Noise protocol handshake and all application data travel inside
     this TLS tunnel, making the connection indistinguishable from normal
     HTTPS browser traffic to DPI middleboxes.

  2. TCP fragmentation – the initial bytes of the TLS ClientHello are split
     into small TCP segments with random inter-segment delays so that
     stateful DPI that only inspects the first segment cannot match known
     signatures.

 Active probing defenses include SNI/ALPN consistency validation, a plausible
 two-certificate chain (fake CA + leaf), and configurable handshake timeouts.