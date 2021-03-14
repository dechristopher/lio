import React, {FC} from 'react'
import {GameVariantCard} from "@components/GameVariantCard/GameVariantCard";
import {bgColors, textColors} from "@utils/styles/colors";
import {GameVariantText} from "@components/GameVariantCard/GameVariantText";
import {FontWeights} from "@utils/constants";
import {ModalContextActions, useModalContext} from "@app/contexts/ModalContext";
import {GameOptions, GameModes} from "@app/components/ModalContent/GameOptions";

/**
 * Content for the create game tab.
 *
 * @returns {Element} - create game tab content
 *
 * @example
 *  <Tabs>
 *      <Tabs.Tab
 *          title="Play Rated Game"
 *          content={<CreateGameTab />}
 *      />
 *  </Tabs>
 */
export const CreateGameTab: FC = () => {
    const [, modalDispatch] = useModalContext()

    return (
        <div className="grid lg:grid-cols-3 grid-cols-1 gap-6 w-full p-6">
            {/* Play Online */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.yellow["300"]}
                textColor={textColors.black["1000"]}
                onClick={() => {
                    modalDispatch({
                        type: ModalContextActions.SetContent,
                        payload: <GameOptions gameMode={GameModes.PlayOnline} />
                    })
                }}
            >
                <GameVariantText
                    timeControl="Play Online"
                    timeControlFontWeight={FontWeights.medium}
                />
            </GameVariantCard>
            {/* Play a Friend */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
                onClick={() => {
                    modalDispatch({
                        type: ModalContextActions.SetContent,
                        payload: <GameOptions gameMode={GameModes.PlayAFriend} />
                    })
                }}
            >
                <GameVariantText
                    timeControl="Play a Friend"
                    timeControlFontWeight={FontWeights.medium}
                />
            </GameVariantCard>
            {/* Play the Computer */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
                onClick={() => {
                    modalDispatch({
                        type: ModalContextActions.SetContent,
                        payload: <GameOptions gameMode={GameModes.PlayComputer} />
                    })
                }}
            >
                <GameVariantText
                    timeControl="Play the Computer"
                    timeControlFontWeight={FontWeights.medium}
                />
            </GameVariantCard>
        </div>
    )
}