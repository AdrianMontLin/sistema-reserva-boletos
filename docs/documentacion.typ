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

// TODO: 2-3 líneas describiendo qué es el sistema y el alcance de esta entrega
// (arquitectura base, solo consulta y reserva, sin tolerancia a fallas)

= Modelos del Sistema

== Modelo Físico/Arquitectónico

// TODO: Explicar cómo está distribuido las partes físicas del sistema (los dispositivos)

== Modelo de Interacción

// TODO: Explicar los estilos de cada interacción según su operación

== Modelo de Fallos

// TODO: Explicar cómo se aplicará el modelo a fallas 

== Modelo de Seguridad

// TODO: Explicar 3 coneptos, Confidencialidad, Integridad y Autenticación/Autorización

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
