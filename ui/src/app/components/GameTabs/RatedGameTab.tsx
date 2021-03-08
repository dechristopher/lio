import React, {FC} from 'react';
import {GameVariantCard} from "@components/GameVariantCard/GameVariantCard";
import {bgColors, textColors} from "@utils/styles/colors";
import {GameVariantText} from "@components/GameVariantCard/GameVariantText";

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
    return (
        <div className="grid xl:grid-cols-4 grid-cols-2 gap-6 w-full p-6">
            {/* Bullet Presets */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.yellow["300"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="¼ + 0"
                    variantName="Bullet"
                />
            </GameVariantCard>
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.yellow["300"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="¼ + 1"
                    variantName="Bullet"
                />
            </GameVariantCard>
            {/* Blitz Presets */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="½ + 0"
                    variantName="Blitz"
                />
            </GameVariantCard>
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="½ + 1"
                    variantName="Blitz"
                />
            </GameVariantCard>
            {/* Rapid Presets */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="1 + 0"
                    variantName="Rapid"
                />
            </GameVariantCard>
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.green["500"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl="1 + 2"
                    variantName="Rapid"
                />
            </GameVariantCard>
            {/* Other Presets */}
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.purple["400"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl=":05 + 0"
                    variantName="Hyper"
                />
            </GameVariantCard>
            <GameVariantCard
                className="w-full"
                bgColor={bgColors.purple["400"]}
                textColor={textColors.black["1000"]}
            >
                <GameVariantText
                    timeControl=":00 ~ 5"
                    variantName="Ulti"
                />
            </GameVariantCard>
        </div>
    )
};