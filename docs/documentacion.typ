#set page(
  paper: "a4",
  margin: (top: 2.5cm, bottom: 2.5cm, left: 3cm, right: 3cm),
  numbering: (current, total) => {
    if current > 1 { str(current - 1) }
  }
)
#set text(font: "Linux Libertine", size: 11pt, lang: "es")
#set par(justify: true, leading: 0.65em)

// --- PORTADA ---
#align(center + horizon)[
  #image("/images/puj-logo.png", width: 30%)
  #v(1cm)
  
  #text(size: 14pt, weight: "bold")[PONTIFICIA UNIVERSIDAD JAVERIANA] \
  #text(size: 11pt, style: "italic")[Facultad de Ingeniería --- Introducción a Sistemas Distribuidos]
  
  #v(2cm)
  #text(size: 22pt, weight: "bold")[
    Diseño del Sistema Reservas de Boletos
  ]

  #v(3cm)
  
  #grid(
    columns: (1fr),
    gutter: 1.2em,
    align(center)[
      *Integrantes:* \
      Elier David Ibarra Medina \
      Samuel Adrian Montaña Linares \
    ]
  )
  
  #v(2cm)
#text(size: 12pt, weight: "bold")[
    Profesor: Rafael Vicente Paez Mendez
  ]
]

#pagebreak() 


= Introducción

El Sistema Reserva de Boletos es una aplicación distribuida que permite a los estudiantes de la universidad consultar eventos culturales y deportivos disponibles en un mes determinado, reservar puestos y modificar sus reservas cambiando el número de puestos o la fecha si el evento ocurre en el mismo mes. El diseño está implementado en 3 capas, Clientes, Lógica de negocio y de Almacenamiento, distribuidas en 3 nodos donde su comunicación es por medio de una librería llamada ZeroMQ, combinando la sincronía para consultas y reservas y, la asíncronía para cancelaciones.

Para la primera entrega, se requieren las operaciones síncronas de consulta de eventos y reserva de puestos funcionando de extremo a extremo entre las 3 capas. El diseño completo de la arquitectura mediante los 4 modelos de sistema y los diagramas de UML. Y por último, la definición del protocolo de pruebas, la estratgia de recolección de métricas y el mecanismo de generación de carga para la evaluación de desempeño.

= Modelos del Sistema

== Modelo Físico

El sistema corre en 3 máquinas distintas una por cada arquitectura:

- *Nodo 1* (_PC personal Samuel Montaña_): Se encarga de ejecutar los procesos cliente, ya que son los que generan las solicitudes de consulta, reserva y cancelación.
- *Nodo 2* (_PC personal Elier Ibarra_): Se encarga de ejecutar los 3 servicios de la capa de negocio, el gestor de transacciones, el servicio RMC y el gestor de cancelaciones.
- *Nodo 3* (_Celular o PC adional-MV_): Se encarga de ejecutar el servicio de persistencia, encargado de guardar y consultar los datos.

Como estos dispositivos no están en la misma red física, se usa Tailscale para crear una red privada virtual que les asigna direcciones IP fijas y los conecta entre sí sin importar dónde estén conectados. Cada servicio tiene un puierto fijo asignado, de tal forma que la comunicación sea siempre predecible. A continuación se muestra en la tabla los componentes del software y sus puertos de comunicación:

#table(
  columns: 3,
  align: left,
  [*Componente*], [*Nodo*], [*Puerto*],
  [Procesos Cliente], [Nodo 1], [(se conecta al puerto 9900 del Nodo 2)],
  [Gestor de Transacciones (canal síncrono)], [Nodo 2], [9900],
  [Gestor de Transacciones (canal asíncrono)], [Nodo 2], [9901],
  [Servicio RMC], [Nodo 2], [9902],
  [Servicio de Cancelaciones y Notificaciones], [Nodo 2], [9903],
  [Servicio de Persistencia], [Nodo 3], [9904],
)


== Modelo de Interacción

Las operarciones de el sistema se componen de dos formas dependiendo de cómo el cliente espera la respuesta. Las Operaciones Síncronas, que hacen referencia a la consultoría de eventos, reserva de puestos y modificación de una reserva. El cliente envía una solicitud y se queda en espera hasta que reciba una respuesta antes de continuar. Acá se implementa el patrón REQ/REP de ZeroMQ, en el cliente y el gestor de transacciones, y como en el gestor de transacciones y el servicio RMC. Y las Operaciones Asíncronas, que hacen referencia a la cancelación de una reserva. El cliente envía una solicitud y recibe de manera inmediata una confirmación de que fue recibida, sin tener que esperar que la cancelación se complete, cuando la cancelación finalmente se procesa en la base de datos, el sistema le avisa al cliente por separado mediante una notificación. Acá se implementa con DEALER/ROUTER para que el cliente no se bloquee al enviar la cancelación, y con PUB/SUB para el envío de la notificación.

== Modelo de Fallos

El sistema se encarga de que si un componente falla, solo deja de responder, no se considera que un componente responda con información correcta o dañada intencionalmente. El Gestor de Transacciones y el servicio RMC, son los puntos donde puede ocurrir una falla, ya que estos son los responsables del flujo principal de la consulta y la reserva.

Para poder determinar si un componente falló, se usa un tiempo límite de espera. Si un proceso no recibe respuesta dentro de un lapso de tiempo razonable, asume que el otro extremo falló. También se considera la posibilidad de que un mensaje se pierda en el camino debido a la red, como puede ser una interrumpción temporal de la conexión, lo cual es distinto a que el proceso realmente haya fallado.  

== Modelo de Seguridad

El sistema para esta primera entrega, no incluye mecanismos de seguridad avanzados, pero hay que recalcar los riesgos que son existentes. Los mensajes que viajan entre los servicios no van cifrados por ZeroMQ, pero al usar Tailscale, si quedan protegidos mientras viajan por la red, ya que esta cifra el tráfico automáticamente. También cualquier proceso conectado a la red Tailscale del equipo podría enviar solicitudes al sistema, ya que no hay ningina verificación de identidad de quién está haciendo la solicitud. Y por último, no se valida que los mensajes lleguen sin ser alterados, más allá de la protección que ya da la red. Estos mecanismos se aceptan dado que el etorno de pruebas es controlado. Como posible mejora futura, se podría agregar el mecanismo de CURVE de ZeroMQ para agregar autenticación de clientes y cifrado adicional.

= Diseño del Sistema

== Diagrama de Despliegue

// TODO: Diagrama de despliegue

== Diagrama de Componentes

// TODO: Diagrama de componentes

== Diagrama de Clases

// TODO: Diagrama de clases

== Diagrama de Secuencia

// TODO: Diagrama de secuencia

= Arquitectura Escalable y Tolerante a Fallos

// TODO: párrafo descriptivo sobre qué componentes se agregarán
// en la Entrega 2 para escalabilidad y tolerancia a fallas
// (ej. réplicas de RMC, réplica de BD, heartbeat/failover)

= Protocolo de Pruebas

// TODO: tipos de prueba a realizar (funcionales, de carga, de concurrencia)

= Obtención de Métricas de Desempeño

// TODO: cómo se medirán tiempo de respuesta y TPM
// (instrumentación en código, librería, herramienta externa — nombrarla)

= Generación de Carga

// TODO: cómo los clientes generan las solicitudes parametrizables
