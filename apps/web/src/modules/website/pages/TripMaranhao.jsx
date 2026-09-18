import { Button, Card, CardBody, Chip } from '@heroui/react'
import { motion as Motion } from 'framer-motion'
import { MapPin, Calendar, Users, Clock, Check, X, Phone, MessageCircle, Star, ChevronRight, Sun, Droplets, Camera, Compass } from 'lucide-react'
import { contacts, createTelUrl, createWhatsAppUrl } from '../data/contacts'

const WHATSAPP_MESSAGE = 'Olá! Gostaria de informações sobre a viagem aos Lençóis Maranhenses'

// Dados do roteiro
const itinerary = [
    {
        day: 1,
        title: 'Embarque e Viagem',
        description: 'Saída de Fraiburgo/SC com destino a Barreirinhas/MA. Viagem em ônibus semi-leito com todo conforto.',
        icon: '🚌',
    },
    {
        day: 2,
        title: 'Chegada em Barreirinhas',
        description: 'Chegada e acomodação na pousada. Tempo livre para descanso e exploração da cidade.',
        icon: '🏨',
    },
    {
        day: 3,
        title: 'Lençóis Maranhenses',
        description: 'Passeio de 4x4 pelos Lençóis Maranhenses. Banho nas lagoas cristalinas: Lagoa Azul e Lagoa Bonita.',
        icon: '🏝️',
    },
    {
        day: 4,
        title: 'Rio Preguiças',
        description: 'Passeio de lancha pelo Rio Preguiças até Atins. Paradas em Vassouras, Mandacaru e Caburé.',
        icon: '🚤',
    },
    {
        day: 5,
        title: 'Dia Livre',
        description: 'Dia livre para passeios opcionais ou relaxar. Opções: Lagoa Bonita, Cardosa, ou artesanato local.',
        icon: '☀️',
    },
    {
        day: 6,
        title: 'Retorno',
        description: 'Início da viagem de retorno para Santa Catarina com lindas memórias.',
        icon: '🏠',
    },
]

// Pacotes
const packages = [
    {
        name: 'Pacote Econômico',
        vehicle: 'Ônibus Convencional',
        duration: '6 dias / 5 noites',
        price: 'Consultar',
        features: ['Transporte ida/volta', 'Hospedagem 3★', 'Café da manhã', 'Passeios inclusos'],
        highlight: false,
    },
    {
        name: 'Pacote Conforto',
        vehicle: 'Semi Leito Panorâmico',
        duration: '6 dias / 5 noites',
        price: 'Consultar',
        features: ['Transporte premium', 'Hospedagem 4★', 'Café + almoço', 'Passeios + guia', 'Kit viagem'],
        highlight: true,
        badge: '⭐ Mais vendido',
    },
    {
        name: 'Pacote Premium',
        vehicle: 'Double Decker',
        duration: '7 dias / 6 noites',
        price: 'Consultar',
        features: ['Double Decker c/ Wi-Fi 4G', 'Hospedagem 4★', 'Pensão completa', 'Todos os passeios', 'Transfer privativo'],
        highlight: false,
    },
]

// O que está incluído
const inclusions = [
    { icon: '🚌', text: 'Transporte ida e volta' },
    { icon: '🏨', text: 'Hospedagem com café' },
    { icon: '🗺️', text: 'Passeios com guia' },
    { icon: '🚗', text: 'Transfers locais' },
    { icon: '🛡️', text: 'Seguro viagem' },
    { icon: '📋', text: 'Suporte 24h' },
]

const exclusions = [
    'Almoço e jantar (exceto onde especificado)',
    'Bebidas alcoólicas',
    'Passeios opcionais',
    'Despesas pessoais',
    'Gorjetas',
]

// FAQ
const faqs = [
    {
        question: 'Qual a melhor época para visitar?',
        answer: 'De junho a setembro, quando as lagoas estão cheias após as chuvas e o sol brilha intensamente.',
    },
    {
        question: 'Preciso de vacina?',
        answer: 'Recomendamos a vacina contra febre amarela, mas não é obrigatória.',
    },
    {
        question: 'O ônibus tem banheiro?',
        answer: 'Sim! Todos os nossos ônibus possuem banheiro, ar-condicionado e poltronas reclináveis.',
    },
    {
        question: 'Posso parcelar?',
        answer: 'Sim! Parcelamos em até 12x no cartão. Consulte condições especiais à vista.',
    },
    {
        question: 'Crianças pagam?',
        answer: 'Crianças até 5 anos no colo não pagam. De 6-12 anos podem ter desconto (consultar).',
    },
]

// Depoimentos específicos Maranhão
const testimonials = [
    {
        name: 'Darlene Xavier',
        text: 'A viagem foi perfeita… Receptivo maravilhoso, motorista maravilhoso, guia maravilhoso. O lugar é simplesmente magnífico e mágico.',
        rating: 5,
    },
    {
        name: 'Carlos Alberto',
        text: 'A experiência no Maranhão foi fantástica e completamente impactante. Os lugares são incríveis e deslumbrantes!',
        rating: 5,
    },
]

export default function TripMaranhao() {
    const handleWhatsApp = (message = WHATSAPP_MESSAGE) => {
        window.open(createWhatsAppUrl(contacts.travelAi, message), '_blank')
    }

    return (
        <div className="bg-white">
            {/* Hero Section */}
            <section className="relative min-h-[70vh] flex items-center overflow-hidden">
                {/* Background Image */}
                <div className="absolute inset-0">
                    <img
                        src="https://images.unsplash.com/photo-1559128010-7c1ad6e1b6a5?w=1920&q=80"
                        alt="Lençóis Maranhenses"
                        className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-r from-dark-900/80 via-dark-900/60 to-transparent" />
                </div>

                {/* Content */}
                <div className="container-max relative z-10 py-20">
                    <Motion.div
                        initial={{ opacity: 0, y: 30 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.8 }}
                        className="max-w-2xl"
                    >
                        <Chip className="bg-gold-500 text-white mb-4">
                            ⭐ Destino mais procurado
                        </Chip>
                        <h1 className="text-4xl md:text-6xl font-heading font-bold text-white mb-6 leading-tight">
                            Lençóis <span className="text-gold-400">Maranhenses</span>
                        </h1>
                        <p className="text-xl text-white/90 mb-8 leading-relaxed">
                            Descubra um dos cenários mais impressionantes do Brasil.
                            Dunas brancas, lagoas cristalinas e paisagens de tirar o fôlego.
                        </p>

                        <div className="flex flex-wrap gap-4 mb-8">
                            <div className="flex items-center gap-2 text-white/80">
                                <Clock size={18} />
                                <span>6-7 dias</span>
                            </div>
                            <div className="flex items-center gap-2 text-white/80">
                                <MapPin size={18} />
                                <span>Barreirinhas, MA</span>
                            </div>
                            <div className="flex items-center gap-2 text-white/80">
                                <Calendar size={18} />
                                <span>Jun - Set</span>
                            </div>
                        </div>

                        <div className="flex flex-wrap gap-4">
                            <Button
                                size="lg"
                                className="bg-gold-500 text-white font-bold hover:bg-gold-600"
                                onClick={() => handleWhatsApp('Olá! Quero reservar a viagem aos Lençóis Maranhenses')}
                            >
                                <MessageCircle size={20} className="mr-2" />
                                Reservar Agora
                            </Button>
                            <Button
                                size="lg"
                                variant="bordered"
                                className="border-white text-white hover:bg-white/10"
                                onClick={() => document.getElementById('pacotes').scrollIntoView({ behavior: 'smooth' })}
                            >
                                Ver Pacotes
                                <ChevronRight size={20} />
                            </Button>
                        </div>
                    </Motion.div>
                </div>
            </section>

            {/* Highlights */}
            <section className="py-12 bg-gold-50 border-y border-gold-100">
                <div className="container-max">
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-8">
                        {[
                            { icon: Sun, label: 'Sol o ano todo', value: '30°C média' },
                            { icon: Droplets, label: 'Lagoas cristalinas', value: '40+ lagoas' },
                            { icon: Camera, label: 'Paisagens únicas', value: 'Insta-worthy' },
                            { icon: Compass, label: 'Aventura completa', value: '4x4 e lancha' },
                        ].map((item, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 20 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                                className="text-center"
                            >
                                <div className="w-14 h-14 mx-auto mb-3 rounded-2xl bg-white shadow-md flex items-center justify-center text-gold-500">
                                    <item.icon size={28} />
                                </div>
                                <div className="font-bold text-dark-900">{item.value}</div>
                                <div className="text-sm text-dark-500">{item.label}</div>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* Roteiro */}
            <section className="section-padding">
                <div className="container-max">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        className="text-center mb-12"
                    >
                        <h2 className="section-title">
                            Roteiro <span className="text-gradient-gold">Completo</span>
                        </h2>
                        <p className="section-subtitle">Cada dia uma nova aventura</p>
                    </Motion.div>

                    <div className="max-w-3xl mx-auto">
                        {itinerary.map((day, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, x: -20 }}
                                whileInView={{ opacity: 1, x: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                                className="flex gap-4 mb-6 last:mb-0"
                            >
                                <div className="flex-shrink-0 w-16 h-16 rounded-2xl bg-gradient-to-br from-gold-400 to-gold-500 flex flex-col items-center justify-center text-white shadow-gold">
                                    <span className="text-xs font-medium">Dia</span>
                                    <span className="text-xl font-bold">{day.day}</span>
                                </div>
                                <Card className="flex-1 border border-light-200 hover:border-gold-300 transition-colors">
                                    <CardBody className="p-4">
                                        <div className="flex items-center gap-2 mb-2">
                                            <span className="text-2xl">{day.icon}</span>
                                            <h3 className="font-bold text-dark-900">{day.title}</h3>
                                        </div>
                                        <p className="text-dark-500 text-sm">{day.description}</p>
                                    </CardBody>
                                </Card>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* Pacotes */}
            <section id="pacotes" className="section-padding bg-light-100">
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
                        <p className="section-subtitle">Opções para todos os perfis</p>
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
                                <Card className={`h-full ${pkg.highlight ? 'ring-2 ring-gold-400 shadow-gold-lg' : 'border border-light-200'}`}>
                                    <CardBody className="p-6">
                                        {pkg.badge && (
                                            <Chip className="bg-gold-500 text-white text-xs mb-4">
                                                {pkg.badge}
                                            </Chip>
                                        )}
                                        <h3 className="text-xl font-bold text-dark-900 mb-2">{pkg.name}</h3>
                                        <p className="text-gold-600 font-medium mb-1">{pkg.vehicle}</p>
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
                                            className={pkg.highlight ? 'bg-gold-500 text-white' : 'bg-dark-900 text-white'}
                                            onClick={() => handleWhatsApp(`Olá! Tenho interesse no ${pkg.name} para Lençóis Maranhenses`)}
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

            {/* O que está incluído / não está */}
            <section className="section-padding">
                <div className="container-max">
                    <div className="grid md:grid-cols-2 gap-12">
                        {/* Incluído */}
                        <Motion.div
                            initial={{ opacity: 0, x: -20 }}
                            whileInView={{ opacity: 1, x: 0 }}
                            viewport={{ once: true }}
                        >
                            <h3 className="text-2xl font-bold text-dark-900 mb-6 flex items-center gap-2">
                                <Check className="text-green-500" /> O que está incluído
                            </h3>
                            <div className="grid grid-cols-2 gap-4">
                                {inclusions.map((item, index) => (
                                    <div key={index} className="flex items-center gap-3 p-3 bg-green-50 rounded-xl border border-green-100">
                                        <span className="text-2xl">{item.icon}</span>
                                        <span className="text-dark-700 text-sm font-medium">{item.text}</span>
                                    </div>
                                ))}
                            </div>
                        </Motion.div>

                        {/* Não incluído */}
                        <Motion.div
                            initial={{ opacity: 0, x: 20 }}
                            whileInView={{ opacity: 1, x: 0 }}
                            viewport={{ once: true }}
                        >
                            <h3 className="text-2xl font-bold text-dark-900 mb-6 flex items-center gap-2">
                                <X className="text-red-400" /> Não incluso
                            </h3>
                            <ul className="space-y-3">
                                {exclusions.map((item, index) => (
                                    <li key={index} className="flex items-center gap-3 text-dark-500">
                                        <div className="w-2 h-2 rounded-full bg-dark-300" />
                                        {item}
                                    </li>
                                ))}
                            </ul>
                        </Motion.div>
                    </div>
                </div>
            </section>

            {/* Depoimentos */}
            <section className="section-padding bg-gold-50">
                <div className="container-max">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        className="text-center mb-12"
                    >
                        <h2 className="section-title">
                            Quem já <span className="text-gradient-gold">foi</span>
                        </h2>
                    </Motion.div>

                    <div className="grid md:grid-cols-2 gap-8 max-w-4xl mx-auto">
                        {testimonials.map((testimonial, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 20 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                            >
                                <Card className="h-full bg-white">
                                    <CardBody className="p-6">
                                        <div className="flex gap-1 mb-4">
                                            {[...Array(testimonial.rating)].map((_, i) => (
                                                <Star key={i} size={18} className="text-gold-400 fill-gold-400" />
                                            ))}
                                        </div>
                                        <p className="text-dark-600 italic mb-4">"{testimonial.text}"</p>
                                        <p className="font-bold text-dark-900">{testimonial.name}</p>
                                    </CardBody>
                                </Card>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* FAQ */}
            <section className="section-padding">
                <div className="container-max max-w-3xl">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        className="text-center mb-12"
                    >
                        <h2 className="section-title">
                            Perguntas <span className="text-gradient-gold">Frequentes</span>
                        </h2>
                    </Motion.div>

                    <div className="space-y-4">
                        {faqs.map((faq, index) => (
                            <Motion.div
                                key={index}
                                initial={{ opacity: 0, y: 10 }}
                                whileInView={{ opacity: 1, y: 0 }}
                                viewport={{ once: true }}
                                transition={{ delay: index * 0.1 }}
                            >
                                <Card className="border border-light-200">
                                    <CardBody className="p-5">
                                        <h4 className="font-bold text-dark-900 mb-2">{faq.question}</h4>
                                        <p className="text-dark-500 text-sm">{faq.answer}</p>
                                    </CardBody>
                                </Card>
                            </Motion.div>
                        ))}
                    </div>
                </div>
            </section>

            {/* CTA Final */}
            <section className="section-padding bg-gradient-to-br from-gold-500 to-gold-600">
                <div className="container-max text-center">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                    >
                        <h2 className="text-3xl md:text-4xl font-heading font-bold text-white mb-4">
                            Pronto para viver essa experiência?
                        </h2>
                        <p className="text-gold-100 text-lg mb-8 max-w-xl mx-auto">
                            Entre em contato agora e garanta sua vaga na próxima viagem aos Lençóis Maranhenses!
                        </p>
                        <div className="flex flex-wrap justify-center gap-4">
                            <Button
                                size="lg"
                                className="bg-white text-gold-600 font-bold hover:bg-gold-50"
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
