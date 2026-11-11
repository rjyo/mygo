import type { Sponsor } from "@/sponsors"

/** A sponsor's banner, light or dark with the site's theme, linking to the sponsor. */
export function SponsorBanner({ sponsor }: { sponsor: Sponsor }) {
  return (
    <a href={sponsor.url} className="block max-w-[600px]">
      <img src={sponsor.light} alt={sponsor.alt} width={600} height={100} loading="lazy" className="w-full dark:hidden" />
      <img src={sponsor.dark} alt={sponsor.alt} width={600} height={100} loading="lazy" className="hidden w-full dark:block" />
    </a>
  )
}
