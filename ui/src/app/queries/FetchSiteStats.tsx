import {BaseURL} from "@utils/constants";

export type SiteStats = {
    playerCount: number;
    activeGames: number;
}

type SiteStatsRes = {
    p: number;
    g: number;
}

/**
 * Fetches various stats like current player and game counts.
 *
 * @returns {Promise<SiteStats>} - site stats
 *
 * @example
 * FetchSiteStats()
 *  .then(siteStats => {})
 *  .catch(err => {})
 */
export const FetchSiteStats = (): Promise<SiteStats> => {
    return new Promise((resolve, reject) => {
        const url = `${BaseURL}/api/stat/site`;
        const req = new Request(url)

        fetch(req, {})
            .then(res => {
                if (res && res.status === 200) {
                    res.json().then((data: SiteStatsRes) => {
                        console.log("Site Stats Res", data)

                        resolve({
                            playerCount: data.p,
                            activeGames: data.g
                        } as SiteStats)
                    })
                        .catch(err => {
                            console.error(`Error parsing response: ${err}`)
                            reject()
                        })
                }
            })
            .catch(err => {
                console.error(`Error fetching site stats: ${err}`)
                reject();
            })
    })
}