import React, {FC, useEffect, useState} from 'react';
import {GameVariantCard} from "@components/GameVariantCard/GameVariantCard";
import {textColors} from "@utils/styles/colors";
import {GameVariantText} from "@components/GameVariantCard/GameVariantText";
import {FetchRatedPools, RatedGame} from "@app/querys/FetchRatedPools";
import {GameModes, GamePools, PoolColors} from "@utils/constants";
import {ModalContextActions, useModalContext} from "@app/contexts/ModalContext";
import {PreGame} from "@app/components/ModalContent/PreGame";
import {Spinner} from "@components/Spinner/Spinner";

/**
 * Content for the rated game tab.
 *
 * @returns {Element} - rated game tab content
 *
 * @example
 *  <Tabs>
 *      <Tabs.Tab
 *          title="Play Rated Game"
 *          content={<RatedGameTab />}
 *      />
 *  </Tabs>
 */
export const RatedGameTab: FC = () => {
    const [, modalDispatch] = useModalContext();

    const [ratedPools, setRatedPools] = useState<RatedGame[]>([])
    const [loading, setLoading] = useState<boolean>(false)

    /**
     * Fetches rated pools on mount and returns a list of games sorted by
     * PoolsOrder.
     */
    useEffect(() => {
        setLoading(true)

        FetchRatedPools()
            .then(ratedPools => {
                setRatedPools(ratedPools);
                setLoading(false)
            })
            .catch(() => setLoading(false))
    }, [])

    return loading ? <Spinner className="my-6"/> :
        <div className="grid xl:grid-cols-4 grid-cols-2 gap-6 w-full p-6">

            {ratedPools.map((game, key) => {
                // the space that separates the time and the variant name
                const lastSpaceIdx = game.name.lastIndexOf(" ");

                return (
                    <GameVariantCard
                        key={key}
                        className="w-full"
                        bgColor={PoolColors[game.group.toUpperCase() as GamePools]}
                        textColor={textColors.black["1000"]}
                        onClick={() => {
                            modalDispatch({
                                type: ModalContextActions.SetContent,
                                payload: <PreGame
                                    gameMode={GameModes.PlayOnline}
                                    gameType={game}
                                />
                            })
                        }}
                    >
                        <GameVariantText
                            timeControl={game.name.substring(0, lastSpaceIdx)}
                            variantName={game.name.substring(lastSpaceIdx)}
                        />
                    </GameVariantCard>
                )
            })}
        </div>
};