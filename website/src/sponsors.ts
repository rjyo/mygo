/**
 * MyGo's sponsors, shown on the home page and at /sponsors. They are the
 * sponsors of README.md: each has the banner it shows in light and dark mode.
 */
export interface Sponsor {
  url: string
  light: string
  dark: string
  alt: string
}

export const sponsors: Sponsor[] = [
  {
    url: "https://openuptime.app/?utm_source=github&utm_medium=sponsor&utm_campaign=mygo",
    light: "https://openuptime.app/sponsor/banner-light.png",
    dark: "https://openuptime.app/sponsor/banner-dark.png",
    alt: "OpenUptime: open-source uptime monitoring and status pages",
  },
]
