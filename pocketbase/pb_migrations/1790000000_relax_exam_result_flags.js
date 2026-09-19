/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("exams")

  collection.fields.addAt(7, new Field({
    "hidden": false,
    "id": "bool2668393089",
    "name": "show_score_after",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  collection.fields.addAt(8, new Field({
    "hidden": false,
    "id": "bool1478399598",
    "name": "show_explanation_after",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  return app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("exams")

  collection.fields.addAt(7, new Field({
    "hidden": false,
    "id": "bool2668393089",
    "name": "show_score_after",
    "presentable": false,
    "required": true,
    "system": false,
    "type": "bool"
  }))

  collection.fields.addAt(8, new Field({
    "hidden": false,
    "id": "bool1478399598",
    "name": "show_explanation_after",
    "presentable": false,
    "required": true,
    "system": false,
    "type": "bool"
  }))

  return app.save(collection)
})
