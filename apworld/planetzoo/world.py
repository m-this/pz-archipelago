from collections.abc import Mapping
from typing import Any

from BaseClasses import ItemClassification
from worlds.AutoWorld import World
from . import options as planetzoo_options
from . import items, locations, regions, rules
from .locations import get_location_names_with_ids, complete_location_list
from .items import get_item_names_with_ids, complete_item_list


class PlanetZooWorld(World):
    """Planet Zoo Randomizer with Archipelago addition"""
    game = "Planet Zoo"  # name of the game/world
    options_dataclass = planetzoo_options.PlanetZooOptions  # options the player can set
    options: planetzoo_options.PlanetZooOptions  # typing hints for option results
    # settings: typing.ClassVar[MyGameSettings]  # will be automatically assigned from type hint
    topology_present = True  # show path to required location checks in spoiler

    location_name_to_id = get_location_names_with_ids(complete_location_list)
    item_name_to_id = get_item_names_with_ids(complete_item_list)

    item_name_groups = {
        "Permits": items.list_of_permits,
    }

    startingmoney = []

    def create_regions(self) -> None:
        regions.create_and_connect_regions(self)
        locations.create_all_locations(self)
        location_count = 0
        for region in self.multiworld.get_regions():
            location_count += len(region.locations)
        print(f"Total locations: {location_count}")

    def set_rules(self) -> None:
        rules.set_all_rules(self)

    def create_items(self) -> None:
        items.create_all_items(self)
        print(f"Total items: {len(self.multiworld.itempool)}")

    def create_item(self, name: str) -> items.PlanetZooItem:
        return items.create_item_with_correct_classification(self, name)

    def get_filler_item_name(self):
        return items.get_random_filler_item_name(self)

    def fill_slot_data(self) -> Mapping[str, Any]:
        GOAL: dict[str, str | dict[str, list[str]]] = {
            "type": "breed",
            "args": {
                "required_breed": ["gpanda"],  # breed a Giant Panda
                "required_research": [],
            },
        }
        data = self.options.as_dict(
            "starting_money", "num_starting_species", "filler_amounts_cash", "filler_amounts_conservation")
        data["goal"] = GOAL
        return data

    def create_event(self, event: str) -> items.PlanetZooItem:
        return items.PlanetZooItem(event, ItemClassification.progression, None, self.player)
