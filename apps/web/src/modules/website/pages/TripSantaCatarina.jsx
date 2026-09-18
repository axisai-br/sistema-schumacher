import { Button, Card, CardBody, Chip } from '@heroui/react'
import { motion as Motion } from 'framer-motion'
import { MapPin, Calendar, Users, Clock, Check, Phone, MessageCircle, Star, ChevronRight, Waves, Sparkles, Palmtree, Heart } from 'lucide-react'
import { contacts, createTelUrl, createWhatsAppUrl } from '../data/contacts'

const WHATSAPP_MESSAGE = 'Olá! Gostaria de informações sobre viagens em Santa Catarina'

// Destinos
const destinations = [
    {
        name: 'Balneário Camboriú',
        image: 'https://images.unsplash.com/photo-1600623471616-8c1966c91ff6?w=800&q=80',
        description: 'A Dubai brasileira! Praias, teleférico, bares na areia e vida noturna.',
        highlights: ['Praia Central', 'Barra Sul', 'Teleférico', 'Roda Gigante'],
    },
    {
        name: 'Beto Carrero World',
        image: 'https://images.unsplash.com/photo-1569154941061-e231b4725ef1?w=800&q=80',
        description: 'O maior parque temático da América Latina. Diversão para toda família!',
        highlights: ['Montanhas-russas', 'Shows ao vivo', 'Zoo', 'Área Kids'],
    },
    {
        name: 'Bombinhas',
        image: 'https://images.unsplash.com/photo-1507525428034-b723cf961d3e?w=800&q=80',
        description: 'Praias paradisíacas com águas cristalinas e trilhas incríveis.',
        highlights: ['Praia de Bombas', 'Trilha da Sepultura', 'Mergulho'],
    },
]

// Pacotes
const packages = [
    {
        name: 'Beto Carrero Express',
        duration: '2 dias / 1 noite',
        price: 'Consultar',
        features: ['Transporte ida/volta', 'Ingresso Beto Carrero', 'Hospedagem', 'Café da manhã'],
        highlight: false,
    },
    {
        name: 'Balneário Completo',
        duration: '4 dias / 3 noites',
        price: 'Consultar',
        features: ['Ônibus semi-leito', 'Hotel 3★ na Barra Sul', 'Café incluído', 'City tour'],
        highlight: true,
        badge: '⭐ Ideal para grupos',
    },
    {
        name: 'Combo SC Total',
        duration: '5 dias / 4 noites',
        price: 'Consultar',
        features: ['Beto Carrero + Balneário', 'Ingresso parque', 'Hospedagem 4★', 'Guia turístico'],
        highlight: false,
    },
]

export default function TripSantaCatarina() {
    const handleWhatsApp = (message = WHATSAPP_MESSAGE) => {
        window.open(createWhatsAppUrl(contacts.travelAi, message), '_blank')
    }

    return (
        <div className="bg-white">
            {/* Hero Section */}
            <section className="relative min-h-[60vh] flex items-center overflow-hidden">
                <div className="absolute inset-0">
                    <img
                        src="https://images.unsplash.com/photo-1600623471616-8c1966c91ff6?w=1920&q=80"
                        alt="Balneário Camboriú"
                        className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-r from-dark-900/80 via-dark-900/60 to-transparent" />
                </div>

                <div className="container-max relative z-10 py-20">
                    <Motion.div
                        initial={{ opacity: 0, y: 30 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.8 }}
                        className="max-w-2xl"
                    >
                        <Chip className="bg-cyan-500 text-white mb-4">
                            🌴 Lazer e diversão
                        </Chip>
                        <h1 className="text-4xl md:text-6xl font-heading font-bold text-white mb-6 leading-tight">
                            Santa <span className="text-cyan-300">Catarina</span>
                        </h1>
                        <p className="text-xl text-white/90 mb-8 leading-relaxed">
                            Praias paradisíacas, diversão no Beto Carrero e a badalação de Balneário Camboriú.
                            Toda a magia do litoral catarinense!
                        </p>

                        <div className="flex flex-wrap gap-4 mb-8">
                            <div className="flex items-center gap-2 text-white/80">
                                <Clock size={18} />
                                <span>2-5 dias</span>
                            </div>
                            <div className="flex items-center gap-2 text-white/80">
                                <MapPin size={18} />
                                <span>Litoral SC</span>
                            </div>
                            <div className="flex items-center gap-2 text-white/80">
                                <Calendar size={18} />
                                <span>Ano todo</span>
                            </div>
                        </div>

                        <div className="flex flex-wrap gap-4">
                            <Button
                                size="lg"
                                className="bg-cyan-500 text-white font-bold hover:bg-cyan-600"
                                onClick={() => handleWhatsApp('Olá! Quero informações sobre viagem a Santa Catarina')}
                            >
                                <MessageCircle size={20} className="mr-2" />
                                Reservar Agora
                            </Button>
                        </div>
                    </Motion.div>
                </div>
            </section>

            {/* Highlights */}
            <section className="py-12 bg-cyan-50 border-y border-cyan-100">
                <div className="container-max">
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-8">
                        {[
                            { icon: Waves, label: 'Praias incríveis', value: '10+ praias' },
                            { icon: Sparkles, label: 'Beto Carrero', value: '#1 Brasil' },
                            { icon: Palmtree, label: 'Balneário', value: 'Bada' },
                            { icon: Heart, label: 'Para família', value: 'Todas idades' },
                        ].map((item, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 20 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                                className="text-center"
                            >
                                <div className="w-14 h-14 mx-auto mb-3 rounded-2xl bg-white shadow-md flex items-center justify-center text-cyan-500">
                                    <item.icon size={28} />
                                </div>
                                <div className="font-bold text-dark-900">{item.value}</div>
                                <div className="text-sm text-dark-500">{item.label}</div>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* Destinos */}
            <section className="section-padding">
                <div className="container-max">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        className="text-center mb-12"
                    >
                        <h2 className="section-title">
                            Destinos <span className="text-gradient-gold">Incríveis</span>
                        </h2>
                        <p className="section-subtitle">Conheça o que te espera</p>
                    </Motion.div>

                    <div className="grid md:grid-cols-3 gap-8">
                        {destinations.map((dest, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 30 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                            >
                                <Card className="overflow-hidden h-full group">
                                    <div className="relative h-48 overflow-hidden">
                                        <img
                                            src={dest.image}
                                            alt={dest.name}
                                            className="w-full h-full object-cover group-hover:scale-110 transition-transform duration-500"
                                        />
                                    </div>
                                    <CardBody className="p-5">
                                        <h3 className="text-xl font-bold text-dark-900 mb-2">{dest.name}</h3>
                                        <p className="text-dark-500 text-sm mb-4">{dest.description}</p>
                                        <div className="flex flex-wrap gap-2">
                                            {dest.highlights.map((item, idx) => (
                                                <Chip key={idx} size="sm" variant="flat" className="bg-cyan-50 text-cyan-700">
                                                    {item}
                                                </Chip>
                                            ))}
                                        </div>
                                    </CardBody>
                                </Card>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* Pacotes */}
            <section className="section-padding bg-light-100">
                <div className="container-max">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        className="text-center mb-12"
                    >
                        <h2 className="section-title">
                            Escolha seu <span className="text-gradient-gold">Pacote</span>
                        </h2>
                    </Motion.div>

                    <div className="grid md:grid-cols-3 gap-8">
                        {packages.map((pkg, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 30 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                            >
                                <Card className={`h-full ${pkg.highlight ? 'ring-2 ring-cyan-400 shadow-lg' : 'border border-light-200'}`}>
                                    <CardBody className="p-6">
                                        {pkg.badge && (
                                            <Chip className="bg-cyan-500 text-white text-xs mb-4">
                                                {pkg.badge}
                                            </Chip>
                                        )}
                                        <h3 className="text-xl font-bold text-dark-900 mb-2">{pkg.name}</h3>
                                        <p className="text-dark-400 text-sm mb-4">{pkg.duration}</p>

                                        <div className="text-3xl font-bold text-dark-900 mb-6">
                                            {pkg.price}
                                        </div>

                                        <ul className="space-y-3 mb-6">
                                            {pkg.features.map((feature, idx) => (
                                                <li key={idx} className="flex items-center gap-2 text-sm">
                                                    <Check size={16} className="text-green-500" />
                                                    <span className="text-dark-600">{feature}</span>
                                                </li>
                                            ))}
                                        </ul>

                                        <Button
                                            fullWidth
                                            className={pkg.highlight ? 'bg-cyan-500 text-white' : 'bg-dark-900 text-white'}
                                            onClick={() => handleWhatsApp(`Olá! Tenho interesse no ${pkg.name}`)}
                                        >
                                            <MessageCircle size={18} className="mr-2" />
                                            Quero esse pacote
                                        </Button>
                                    </CardBody>
                                </Card>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* CTA Final */}
            <section className="section-padding bg-gradient-to-br from-cyan-500 to-cyan-600">
                <div className="container-max text-center">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                    >
                        <h2 className="text-3xl md:text-4xl font-heading font-bold text-white mb-4">
                            Bora para Santa Catarina?
                        </h2>
                        <p className="text-cyan-100 text-lg mb-8 max-w-xl mx-auto">
                            Entre em contato e monte o pacote perfeito para você e sua família!
                        </p>
                        <div className="flex flex-wrap justify-center gap-4">
                            <Button
                                size="lg"
                                className="bg-white text-cyan-600 font-bold hover:bg-cyan-50"
                                onClick={() => handleWhatsApp()}
                            >
                                <MessageCircle size={20} className="mr-2" />
                                WhatsApp
                            </Button>
                            <Button
                                size="lg"
                                variant="bordered"
                                className="border-white text-white hover:bg-white/10"
                                onClick={() => window.location.href = createTelUrl(contacts.legacyVoice)}
                            >
                                <Phone size={20} className="mr-2" />
                                {contacts.legacyVoice.display}
                            </Button>
                        </div>
                    </Motion.div>
                </div>
            </section>
        </div>
    )
}
