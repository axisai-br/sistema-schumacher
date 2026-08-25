import { useEffect, useRef, useState } from 'react'

const ANIMATED_BUS_SOURCE = '/assets/bus.webp'
const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)'
const REDUCED_MOTION_MEDIA_QUERY = '(prefers-reduced-motion: reduce)'

function useMediaQuery(query) {
    const [matches, setMatches] = useState(() => window.matchMedia(query).matches)

    useEffect(() => {
        const mediaQuery = window.matchMedia(query)
        const updateMatch = () => setMatches(mediaQuery.matches)

        updateMatch()
        mediaQuery.addEventListener('change', updateMatch)

        return () => mediaQuery.removeEventListener('change', updateMatch)
    }, [query])

    return matches
}

function drawFrameCover(canvas, frame) {
    const context = canvas.getContext('2d')
    if (!context || !frame || canvas.width === 0 || canvas.height === 0) return

    const frameWidth = frame.displayWidth || frame.width
    const frameHeight = frame.displayHeight || frame.height
    const scale = Math.max(canvas.width / frameWidth, canvas.height / frameHeight)
    const width = frameWidth * scale
    const height = frameHeight * scale

    context.clearRect(0, 0, canvas.width, canvas.height)
    context.drawImage(
        frame,
        (canvas.width - width) / 2,
        (canvas.height - height) / 2,
        width,
        height,
    )
}

function StaticBusSection() {
    return (
        <section
            className="relative flex h-[70svh] min-h-[30rem] max-h-[48rem] items-center justify-center overflow-hidden border-t border-brand-blue-grey/30 bg-brand-gunship"
            data-bus-mode="static"
        >
            <div className="absolute inset-x-0 top-0 h-2 bg-brand-ember" aria-hidden="true" />
            <div className="px-4 text-center">
                <h2 className="mb-3 text-3xl font-bold text-brand-lynx sm:text-5xl">
                    Sua Jornada Começa Aqui
                </h2>
                <p className="text-base font-medium text-brand-lynx sm:text-lg">↓ Continue descendo ↓</p>
            </div>
        </section>
    )
}

export default function BusScrollAnimation() {
    const sectionRef = useRef(null)
    const canvasRef = useRef(null)
    const framesRef = useRef([])
    const currentFrameRef = useRef(0)
    const isDesktop = useMediaQuery(DESKTOP_MEDIA_QUERY)
    const prefersReducedMotion = useMediaQuery(REDUCED_MOTION_MEDIA_QUERY)
    const shouldAnimate = isDesktop && !prefersReducedMotion
    const [isLoading, setIsLoading] = useState(shouldAnimate)
    const [hasError, setHasError] = useState(false)

    useEffect(() => {
        if (!shouldAnimate) {
            setIsLoading(false)
            setHasError(false)
            return undefined
        }

        const abortController = new AbortController()
        let decoder
        let animationFrame
        let isDisposed = false

        const closeResource = (resource) => {
            try {
                resource?.close()
            } catch {
                // The resource may already have been closed by the browser.
            }
        }

        const closeDecoder = () => {
            const activeDecoder = decoder
            decoder = undefined
            closeResource(activeDecoder)
        }

        setIsLoading(true)
        setHasError(false)

        const renderCurrentFrame = () => {
            const canvas = canvasRef.current
            const frame = framesRef.current[currentFrameRef.current]
            if (canvas && frame) drawFrameCover(canvas, frame)
        }

        const resizeCanvas = () => {
            const canvas = canvasRef.current
            if (!canvas) return

            const bounds = canvas.getBoundingClientRect()
            const sourceFrame = framesRef.current[0]
            const sourceWidth = sourceFrame?.displayWidth || sourceFrame?.width || bounds.width
            const sourceHeight = sourceFrame?.displayHeight || sourceFrame?.height || bounds.height
            const pixelRatio = Math.min(
                window.devicePixelRatio || 1,
                sourceWidth / bounds.width,
                sourceHeight / bounds.height,
            )
            canvas.width = Math.max(1, Math.round(bounds.width * pixelRatio))
            canvas.height = Math.max(1, Math.round(bounds.height * pixelRatio))
            renderCurrentFrame()
        }

        const renderScrollPosition = () => {
            animationFrame = undefined

            const section = sectionRef.current
            if (!section || framesRef.current.length === 0) return

            const scrollDistance = -section.getBoundingClientRect().top
            const maxScroll = section.offsetHeight - window.innerHeight
            const progress = maxScroll > 0
                ? Math.max(0, Math.min(1, scrollDistance / maxScroll))
                : 0

            currentFrameRef.current = Math.round(progress * (framesRef.current.length - 1))
            renderCurrentFrame()
        }

        const requestRender = () => {
            if (animationFrame === undefined) {
                animationFrame = window.requestAnimationFrame(renderScrollPosition)
            }
        }

        const handleResize = () => {
            resizeCanvas()
            requestRender()
        }

        async function loadFrames() {
            const decodedFrames = []
            const closeDecodedFrames = () => {
                decodedFrames.splice(0).forEach(closeResource)
            }

            try {
                if (!('ImageDecoder' in window)) {
                    throw new Error('unsupported-decoder')
                }

                const response = await fetch(ANIMATED_BUS_SOURCE, {
                    signal: abortController.signal,
                })
                if (!response.ok) throw new Error('animation-unavailable')

                decoder = new window.ImageDecoder({
                    data: await response.arrayBuffer(),
                    type: 'image/webp',
                })

                await decoder.tracks.ready
                const frameCount = decoder.tracks.selectedTrack?.frameCount || 0
                if (frameCount === 0) throw new Error('animation-empty')

                for (let frameIndex = 0; frameIndex < frameCount; frameIndex += 1) {
                    if (isDisposed) {
                        closeDecodedFrames()
                        return
                    }
                    const { image } = await decoder.decode({ frameIndex })
                    decodedFrames.push(image)
                }

                if (isDisposed) {
                    closeDecodedFrames()
                    return
                }

                framesRef.current = decodedFrames
                currentFrameRef.current = 0
                resizeCanvas()
                renderScrollPosition()
                window.addEventListener('resize', handleResize)
                window.addEventListener('scroll', requestRender, { passive: true })
                setIsLoading(false)
            } catch (error) {
                closeDecodedFrames()
                closeDecoder()
                if (!isDisposed && error.name !== 'AbortError') {
                    setHasError(true)
                    setIsLoading(false)
                }
            }
        }

        loadFrames()

        return () => {
            isDisposed = true
            abortController.abort()
            window.removeEventListener('resize', handleResize)
            window.removeEventListener('scroll', requestRender)
            if (animationFrame !== undefined) {
                window.cancelAnimationFrame(animationFrame)
            }
            closeDecoder()
            framesRef.current.forEach(closeResource)
            framesRef.current = []
        }
    }, [shouldAnimate])

    if (!shouldAnimate || hasError) {
        return <StaticBusSection />
    }

    return (
        <section
            ref={sectionRef}
            className="relative h-[500vh] bg-white"
            data-bus-mode="animated"
        >
            <div className="sticky top-0 flex h-screen w-full items-center justify-center overflow-hidden">
                {isLoading && (
                    <div
                        aria-label="Carregando animação do ônibus"
                        className="h-16 w-16 animate-spin rounded-full border-b-2 border-t-2 border-gold-400"
                        role="status"
                    />
                )}

                <canvas
                    ref={canvasRef}
                    aria-label="Ônibus Schumacher Tur em movimento"
                    className={`h-full w-full ${isLoading ? 'invisible' : 'block'}`}
                    role="img"
                />

                <div className="absolute inset-0 bg-gradient-to-t from-white via-transparent to-transparent pointer-events-none" />
                <div className="absolute bottom-16 left-4 right-4 text-center pointer-events-none">
                    <h2 className="mb-4 text-4xl font-bold text-gradient-gold drop-shadow-lg sm:text-5xl lg:text-6xl">
                        Sua Jornada Começa Aqui
                    </h2>
                    <p className="text-lg font-medium text-gold-500">↓ Continue descendo ↓</p>
                </div>
            </div>
        </section>
    )
}
