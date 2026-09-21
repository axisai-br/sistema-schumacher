export const featuredServices = [
    {
        id: 'fretamento-empresarial',
        title: 'Fretamento Empresarial',
        description: 'Transporte regular de colaboradores com rotas personalizadas e pontualidade garantida.',
        features: ['Rotas customizadas', 'Contratos flexíveis'],
    },
    {
        id: 'eventos-transfers',
        title: 'Eventos e Transfers',
        description: 'Transporte para eventos corporativos, casamentos, formaturas e ocasiões especiais.',
        features: ['Logística completa', 'Atendimento VIP'],
    },
]

const [business, events] = featuredServices

export const serviceCatalog = [
    { title: business.title, featuredId: business.id, summary: 'Transporte regular de colaboradores.' },
    { title: 'Familiar' },
    { title: 'Para grupos' },
    { title: 'Escolar' },
    { title: 'Excursões' },
    { title: 'Esportivo' },
    { title: 'Eventos', featuredId: events.id, summary: 'Transporte para eventos corporativos, casamentos e formaturas.' },
    { title: 'Transfers', featuredId: events.id, summary: 'Confira o destaque de Eventos e Transfers.' },
    {
        title: 'Turismo',
        destinations: [
            { title: 'Lençóis Maranhenses', to: '/viagens/maranhao' },
            { title: 'Santa Catarina', to: '/viagens/santa-catarina' },
        ],
    },
]
