import {BaseURL, GamePools, PoolsOrder} from "@utils/constants";

// Rated game structure.
export type RatedGame = {
    name: string;
    group: string;
    time: {
        t: number;
        i: number;
        d: number;
    }
}

// Response returned by the backend.
export type RatedPoolsRes = {
    [GamePools.Bullet]: RatedGame[],
    [GamePools.Blitz]: RatedGame[],
    [GamePools.Rapid]: RatedGame[],
    [GamePools.Hyper]: RatedGame[],
    [GamePools.Ulti]: RatedGame[],
}

/**
 * Fetches all available rated pools and returns an array of rated games
 * in an order defined by PoolsOrder.
 *
 * @returns {Promise<RatedGame[]>} - sorted list of rated games
 *
 * @example
 * FetchRatedPools()
 *  .then(ratedGames => {})
 *  .catch(err => {})
 */
export const FetchRatedPools = (): Promise<RatedGame[]> => {
    return new Promise((resolve, reject) => {
        const url = `http://${BaseURL}/api/pools`;
        const req = new Request(url)

        fetch(req, {})
            .then(res => {
                if (res && res.status === 200) {
                    res.json().then((data: RatedPoolsRes) => {
                        console.log("Rated Pools Res", data)
                        const pools: RatedGame[] = [];

                        // loop over the rated pool objects and store their games in an array
                        Object.keys(data).forEach(key => {
                            pools.push(...data[key as keyof RatedPoolsRes])
                        })

                        // sort the array using the order we defined and return it
                        resolve(sortPools(pools, PoolsOrder))
                    })
                    .catch(err => {
                        console.error(`Error parsing response: ${err}`)
                        reject()
                    })
                }
            })
            .catch(err => {
                console.error(`Error fetching rated pools: ${err}`)
                reject();
            })
    })
}

/**
 * Sorts an array of rated games in a specific order.
 *
 * @param {RatedGame[]} games - rated games to sort
 * @param {GamePools} poolsOrder - order in which we should order our rated games
 * @returns {RatedGame[]} - sorted list of rated games
 *
 * @example
 * sortPools(gamesToSort, gameOrder);
 */
const sortPools = (games: RatedGame[], poolsOrder: GamePools[]): RatedGame[] => {
    games.sort((a, b) => {
        const groupA = a.group.toUpperCase(), groupB = b.group.toUpperCase();

        if (poolsOrder.indexOf(groupA as GamePools) > poolsOrder.indexOf(groupB as GamePools)) {
            return 1;
        } else {
            return -1;
        }
    });

    return games;
};